package main

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Feedback struct {
	ID      int
	Author  string
	Message string
}

type PageData struct {
	Feedbacks []Feedback
	Total     int
	Error     string
	Author    string
	Message   string
}

var (
	feedbacks      []Feedback
	nextFeedbackID = 1
	feedbackMu     sync.RWMutex
)

// Шаблоны и CSS включаются в программу при сборке.
//
//go:embed templates/*.html static/*.css
var assets embed.FS

var pages = template.Must(template.ParseFS(assets, "templates/*.html"))

func routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", homeHandler)
	mux.HandleFunc("/feedback", feedbackHandler)
	mux.HandleFunc("/about", aboutHandler)
	staticFiles, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFiles))))
	return mux
}

func main() {
	server := &http.Server{
		Addr:              "localhost:8080",
		Handler:           routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Println("Feedback Collector: http://localhost:8080 — откройте адрес в браузере. Остановка: Ctrl+C.")
	log.Fatal(server.ListenAndServe())
}

func feedbackPageData() PageData {
	feedbackMu.RLock()
	defer feedbackMu.RUnlock()
	return PageData{Feedbacks: append([]Feedback(nil), feedbacks...), Total: len(feedbacks)}
}

func render(w http.ResponseWriter, name string, status int, data PageData) {
	var body bytes.Buffer
	if err := pages.ExecuteTemplate(&body, name, data); err != nil {
		log.Println("Ошибка шаблона:", err)
		http.Error(w, "Не удалось отобразить страницу.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		w.Header().Set("Allow", method)
		http.Error(w, "Метод не поддерживается. Используйте "+method+".", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	render(w, "index.html", http.StatusOK, feedbackPageData())
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	render(w, "about.html", http.StatusOK, PageData{})
}

func feedbackHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		data := feedbackPageData()
		data.Error = "Не удалось прочитать форму. Проверьте отправленные данные."
		render(w, "index.html", http.StatusBadRequest, data)
		return
	}
	// FormValue читает значения полей HTML-формы.
	author := strings.TrimSpace(r.FormValue("author"))
	message := strings.TrimSpace(r.FormValue("message"))
	if author == "" || message == "" {
		data := feedbackPageData()
		data.Error = "Укажите имя автора и текст отзыва. Поля не могут быть пустыми или состоять только из пробелов."
		data.Author, data.Message = author, message
		render(w, "index.html", http.StatusBadRequest, data)
		return
	}
	feedbackMu.Lock()
	feedbacks = append(feedbacks, Feedback{ID: nextFeedbackID, Author: author, Message: message})
	nextFeedbackID++
	feedbackMu.Unlock()
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
