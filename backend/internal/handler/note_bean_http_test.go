package handler_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/wjecoffeetaste/wjecoffeetaste/internal/handler"
	"github.com/wjecoffeetaste/wjecoffeetaste/internal/middleware"
	"github.com/wjecoffeetaste/wjecoffeetaste/internal/model"
	"github.com/wjecoffeetaste/wjecoffeetaste/internal/repository"
	"github.com/wjecoffeetaste/wjecoffeetaste/internal/service"
	"github.com/wjecoffeetaste/wjecoffeetaste/internal/util"
)

func newHandlerTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

type env struct {
	db  *gorm.DB
	r   *gin.Engine
	bid uint
}

func setupEnv(t *testing.T) *env {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.TastingNote{}, &model.BrewRecipe{},
		&model.CoffeeBean{}, &model.Comment{}, &model.Like{}, &model.UserFollow{},
	); err != nil {
		t.Fatal(err)
	}
	logger := newHandlerTestLogger()
	noteRepo := repository.NewTastingNoteRepository(db)
	beanRepo := repository.NewCoffeeBeanRepository(db)
	likeRepo := repository.NewLikeRepository(db)
	noteSvc := service.NewNoteService(noteRepo, beanRepo, logger)
	beanSvc := service.NewBeanService(beanRepo, logger)
	likeSvc := service.NewLikeService(likeRepo, noteRepo, logger)
	noteH := handler.NewNoteHandler(noteSvc, likeSvc, logger)
	beanH := handler.NewBeanHandler(beanSvc, logger)

	u := &model.User{Username: "owner", Email: "o@x", PasswordHash: "x"}
	if err := db.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	b := &model.CoffeeBean{Name: "豆子A", Origin: "产地A", ProcessMethod: "washed"}
	if err := db.Create(b).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler(logger))
	r.Use(func(c *gin.Context) {
		c.Set(middleware.UserKey, &util.Claims{UserID: u.ID, Username: u.Username})
		c.Next()
	})
	r.POST("/notes", noteH.Create)
	r.PUT("/notes/:id", noteH.Update)
	r.GET("/notes/:id", noteH.Get)
	r.DELETE("/beans/:id", beanH.Delete)
	r.GET("/beans", beanH.List)
	return &env{db: db, r: r, bid: b.ID}
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		raw, _ := json.Marshal(body)
		buf.Write(raw)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestNoteCreateUpdateBindLifecycle(t *testing.T) {
	e := setupEnv(t)

	// Create bound to bean.
	w := doJSON(t, e.r, http.MethodPost, "/notes", map[string]interface{}{
		"coffee_bean_id": e.bid,
		"coffee_name":    "豆子A",
		"origin":         "旧产地",
		"roast_level":    "light",
		"notes_text":     "原文",
		"overall_score":  9,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	var created struct {
		Data model.TastingNote `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	if created.Data.CoffeeBeanID == nil || *created.Data.CoffeeBeanID != e.bid {
		t.Fatalf("create should bind bean, got %v", created.Data.CoffeeBeanID)
	}
	noteID := created.Data.ID

	// Create with non-existent bean -> 422.
	w = doJSON(t, e.r, http.MethodPost, "/notes", map[string]interface{}{
		"coffee_bean_id": 424242,
		"coffee_name":    "x",
		"roast_level":    "light",
	})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing bean, got %d", w.Code)
	}

	// Admin renames the bean.
	if err := e.db.Model(&model.CoffeeBean{}).Where("id = ?", e.bid).
		Updates(map[string]interface{}{"name": "豆子A新名", "origin": "新产地", "process_method": "natural"}).Error; err != nil {
		t.Fatal(err)
	}

	// Detail must show the live bean name and untouched note text/score.
	w = doJSON(t, e.r, http.MethodGet, "/notes/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get status=%d", w.Code)
	}
	var detail struct {
		Data struct {
			Note model.TastingNote `json:"note"`
			Bean struct {
				Name          string `json:"name"`
				Origin        string `json:"origin"`
				ProcessMethod string `json:"process_method"`
			} `json:"bean"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &detail)
	if detail.Data.Bean.Name != "豆子A新名" || detail.Data.Bean.Origin != "新产地" || detail.Data.Bean.ProcessMethod != "natural" {
		t.Fatalf("bean info should reflect rename, got %+v", detail.Data.Bean)
	}
	if detail.Data.Note.CoffeeName != "豆子A" || detail.Data.Note.Origin != "旧产地" || detail.Data.Note.NotesText != "原文" || detail.Data.Note.OverallScore != 9 {
		t.Fatalf("note snapshot must not change, got name=%s origin=%s text=%s score=%v",
			detail.Data.Note.CoffeeName, detail.Data.Note.Origin, detail.Data.Note.NotesText, detail.Data.Note.OverallScore)
	}

	// Delete the referenced bean -> 409 with count.
	w = doJSON(t, e.r, http.MethodDelete, "/beans/1", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 deleting referenced bean, got %d body=%s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("1 tasting note(s)")) {
		t.Fatalf("409 message must state count, body=%s", w.Body.String())
	}

	// Edit note: unbind via coffee_bean_id=0.
	w = doJSON(t, e.r, http.MethodPut, "/notes/1", map[string]interface{}{
		"coffee_bean_id": 0,
		"coffee_name":    "豆子A",
		"roast_level":    "light",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("unbind update status=%d body=%s", w.Code, w.Body.String())
	}
	var updated struct {
		Data model.TastingNote `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.Data.CoffeeBeanID != nil {
		t.Fatalf("note should be unbound, got %v", *updated.Data.CoffeeBeanID)
	}

	// Detail without bean must omit bean field and fall back to snapshot name.
	w = doJSON(t, e.r, http.MethodGet, "/notes/1", nil)
	if bytes.Contains(w.Body.Bytes(), []byte(`"bean"`)) {
		t.Fatalf("unbound note detail must not include bean: %s", w.Body.String())
	}

	// Rebind and then bean delete is still blocked; after note deletion bean delete works.
	w = doJSON(t, e.r, http.MethodPut, "/notes/1", map[string]interface{}{
		"coffee_bean_id": e.bid,
		"coffee_name":    "豆子A",
		"roast_level":    "light",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("rebind status=%d", w.Code)
	}
	if err := e.db.Delete(&model.TastingNote{}, noteID).Error; err != nil {
		t.Fatal(err)
	}
	w = doJSON(t, e.r, http.MethodDelete, "/beans/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("unreferenced bean delete should succeed, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestBeanListIncludesNoteCount(t *testing.T) {
	e := setupEnv(t)
	_ = doJSON(t, e.r, http.MethodPost, "/notes", map[string]interface{}{
		"coffee_bean_id": e.bid, "coffee_name": "豆子A", "roast_level": "light",
	})
	w := doJSON(t, e.r, http.MethodGet, "/beans", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list status=%d", w.Code)
	}
	var page struct {
		Data struct {
			List []struct {
				Name      string `json:"name"`
				NoteCount int64  `json:"note_count"`
			} `json:"list"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &page)
	found := false
	for _, b := range page.Data.List {
		if b.Name == "豆子A" {
			found = true
			if b.NoteCount != 1 {
				t.Fatalf("note_count should be 1, got %d", b.NoteCount)
			}
		}
	}
	if !found {
		t.Fatal("bean not present in list")
	}
}
