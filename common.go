//go:generate sqlc generate --file db/sqlc.yaml
package main

// This is an experiment

// This file should be copied to new projects that needs connection to DB, HTTP
// server, views and helpers setup. So it should do as much work as possible
// just by including it in the project. I imagine that I will use it by copying
// the code instead of referencing it. and changing the constants to what I
// think is suitable for the new project.

// HOW TO USE

// 1. Copy common.go, .env.sample, sqlc.yaml
// 2. Write queries in query.sql and use `go generate` to generate functions with sqlc
// 3. Use `router` to add your gorilla routes, or shorthand methods GET, POST, DELETE...etc
// 4. Add Helpers to `helpers` map
// 5. call `Start()` to start the server

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "embed"

	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

const (
	APP_NAME                = "library"
	MAX_DB_OPEN_CONNECTIONS = 5
	MAX_DB_IDLE_CONNECTIONS = 5
	STATIC_DIR_PATH         = "public"
	BIND_ADDRESS            = "0.0.0.0:3000"
	VIEWS_EXTENSION         = ".html"
	SESSION_COOKIE_NAME     = APP_NAME + "_session"
	CSRF_COOKIE_NAME        = APP_NAME + "_csrf"
)

var (
	Q       *Queries
	router  *mux.Router
	session *sessions.CookieStore

	VARS = mux.Vars
	CSRF = csrf.TemplateField
)

// Some aliases to make it easier
type Response = http.ResponseWriter
type Request = *http.Request
type Output = http.HandlerFunc
type Locals map[string]interface{} // passed to views/templates

const (
	_        = iota
	KB int64 = 1 << (10 * iota)
	MB
	GB
	TB
	PB
)

func init() {
	log.SetFlags(log.Ltime)

	db, err := sqlx.Connect("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	db.SetMaxOpenConns(MAX_DB_OPEN_CONNECTIONS)
	db.SetMaxIdleConns(MAX_DB_IDLE_CONNECTIONS)

	Q = New(queryLogger{db})
	router = mux.NewRouter()
	session = sessions.NewCookieStore([]byte(os.Getenv("SESSION_SECRET")))
	session.Options.HttpOnly = true
}

func Start() {
	compileViews()
	middlewares := []func(http.Handler) http.Handler{
		methodOverrideHandler,
		csrf.Protect(
			[]byte(os.Getenv("SESSION_SECRET")),
			csrf.Path("/"),
			csrf.FieldName("csrf"),
			csrf.CookieName(CSRF_COOKIE_NAME),
		),
		RequestLoggerHandler,
	}

	router.PathPrefix("/").Handler(staticWithoutDirectoryListingHandler())
	var handler http.Handler = router
	for _, v := range middlewares {
		handler = v(handler)
	}

	http.Handle("/", handler)

	srv := &http.Server{
		Handler:      handler,
		Addr:         BIND_ADDRESS,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Printf("Starting server: %s", BIND_ADDRESS)
	log.Fatal(srv.ListenAndServe())
}

// LOGGING ===============================================

const (
	DEBUG = "\033[97;42m"
	INFO  = "\033[97;43m"
)

func Log(level, label, text string, args ...interface{}) func() {
	start := time.Now()
	return func() {
		if len(args) > 0 {
			log.Printf("%s %s \033[0m (%s) %s %v", level, label, time.Now().Sub(start), text, args)
		} else {
			log.Printf("%s %s \033[0m (%s) %s", level, label, time.Now().Sub(start), text)
		}
	}
}

// DATABASE CONNECTION ===================================

type queryLogger struct {
	db *sqlx.DB
}

func (p queryLogger) ExecContext(ctx context.Context, q string, args ...interface{}) (sql.Result, error) {
	defer Log(DEBUG, "DB Exec", q, args)()
	return p.db.ExecContext(ctx, q, args...)
}
func (p queryLogger) PrepareContext(ctx context.Context, q string) (*sql.Stmt, error) {
	return p.db.PrepareContext(ctx, q)
}
func (p queryLogger) QueryContext(ctx context.Context, q string, args ...interface{}) (*sql.Rows, error) {
	defer Log(DEBUG, "DB Query", q, args)()
	return p.db.QueryContext(ctx, q, args...)
}
func (p queryLogger) QueryRowContext(ctx context.Context, q string, args ...interface{}) *sql.Row {
	defer Log(DEBUG, "DB Row", q, args)()
	return p.db.QueryRowContext(ctx, q, args...)
}

// ROUTES HELPERS ==========================================

type HandlerFunc func(http.ResponseWriter, *http.Request) http.HandlerFunc

func handlerFuncToHttpHandler(handler HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handler(w, r)(w, r)
	}
}

func NotFound(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "", http.StatusNotFound)
}

func BadRequest(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "", http.StatusBadRequest)
}

func Unauthorized(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "", http.StatusUnauthorized)
}

func InternalServerError(err error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func Redirect(url string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, url, http.StatusFound)
	}
}

func GET(path string, handler HandlerFunc, middlewares ...func(http.HandlerFunc) http.HandlerFunc) {
	router.Methods("GET").Path(path).HandlerFunc(applyMiddlewares(handlerFuncToHttpHandler(handler), middlewares...))
}

func POST(path string, handler HandlerFunc, middlewares ...func(http.HandlerFunc) http.HandlerFunc) {
	router.Methods("POST").Path(path).HandlerFunc(applyMiddlewares(handlerFuncToHttpHandler(handler), middlewares...))
}

func DELETE(path string, handler HandlerFunc, middlewares ...func(http.HandlerFunc) http.HandlerFunc) {
	router.Methods("DELETE").Path(path).HandlerFunc(applyMiddlewares(handlerFuncToHttpHandler(handler), middlewares...))
}

// VIEWS ====================

//go:embed views
var views embed.FS
var templates *template.Template
var helpers = template.FuncMap{}

func compileViews() {
	templates = template.New("")
	fs.WalkDir(views, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, VIEWS_EXTENSION) && d.Type().IsRegular() {
			name := strings.TrimPrefix(path, "views/")
			name = strings.TrimSuffix(name, VIEWS_EXTENSION)
			defer Log(DEBUG, "View", name)()

			c, err := fs.ReadFile(views, path)
			if err != nil {
				return err
			}

			template.Must(templates.New(name).Funcs(helpers).Parse(string(c)))
		}

		return nil
	})
}

func partial(path string, data interface{}) string {
	v := templates.Lookup(path)
	if v == nil {
		return fmt.Sprintf("view %s not found", path)
	}

	w := bytes.NewBufferString("")
	err := v.Execute(w, data)
	if err != nil {
		return "rendering error " + path + " " + err.Error()
	}

	return w.String()
}

func Render(path string, view string, data Locals) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data["view"] = view
		data["request"] = r
		fmt.Fprint(w, partial(path, data))
	}
}

func HELPER(name string, f interface{}) {
	if _, ok := helpers[name]; ok {
		log.Fatalf("Helper: %s has been defined already", name)
	}

	helpers[name] = f
}

// SESSION =================================

func SESSION(r *http.Request) *sessions.Session {
	s, _ := session.Get(r, SESSION_COOKIE_NAME)
	return s
}

// HANDLERS MIDDLEWARES =============================

// First middleware gets executed first
func applyMiddlewares(handler http.HandlerFunc, middlewares ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// SERVER MIDDLEWARES ==============================

func staticWithoutDirectoryListingHandler() http.Handler {
	dir := http.Dir(STATIC_DIR_PATH)
	server := http.FileServer(dir)
	handler := http.StripPrefix("/", server)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

// Derived from Gorilla middleware https://github.com/gorilla/handlers/blob/v1.5.1/handlers.go#L134
func methodOverrideHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			om := r.FormValue("_method")
			if om == "PUT" || om == "PATCH" || om == "DELETE" {
				r.Method = om
			}
		}
		h.ServeHTTP(w, r)
	})
}

func RequestLoggerHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer Log(INFO, r.Method, r.URL.Path)()
		h.ServeHTTP(w, r)
	})
}

// HELPERS FUNCTIONS ======================

func atoi32(s string) int32 {
	i, _ := strconv.ParseInt(s, 10, 32)
	return int32(i)
}

func atoi64(s string) int64 {
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}

func NullString(s string) sql.NullString {
	return sql.NullString{
		String: s,
		Valid:  len(s) > 0,
	}
}

// VALIDATION ============================

type ValidationErrors map[string][]error

func (v ValidationErrors) Add(field string, err error) {
	v[field] = append(v[field], err)
}

func ValidateStringPresent(val, key, label string, ve ValidationErrors) {
	if len(strings.TrimSpace(val)) == 0 {
		ve.Add(key, fmt.Errorf("%s can't be empty", label))
	}
}

func ValidateStringLength(val, key, label string, ve ValidationErrors, min, max int) {
	l := len(strings.TrimSpace(val))
	if l < min || l > max {
		ve.Add(key, fmt.Errorf("%s has to be between %d and %d characters, length is %d", label, min, max, l))
	}
}

func ValidateStringNumeric(val, key, label string, ve ValidationErrors) {
	for _, c := range val {
		if !strings.ContainsRune("0123456789", c) {
			ve.Add(key, fmt.Errorf("%s has to consist of numbers", label))
			return
		}
	}
}

func ValidateISBN13(val, key, label string, ve ValidationErrors) {
	if len(val) != 13 {
		ve.Add(key, fmt.Errorf("%s has to be 13 digits", label))
		return
	}

	sum := 0
	for i, s := range val {
		digit, _ := strconv.Atoi(string(s))
		if i%2 == 0 {
			sum += digit
		} else {
			sum += digit * 3
		}
	}

	if sum%10 != 0 {
		ve.Add(key, fmt.Errorf("%s is not a valid ISBN13 number", label))
	}
}

func ValidateImage(val io.Reader, key, label string, ve ValidationErrors, maxw, maxh int) {
	if val == nil {
		return
	}

	image, _, err := image.Decode(val)
	if err != nil {
		ve.Add(key, fmt.Errorf("%s has an unsupported format supported formats are JPG, GIF, PNG", label))
		return
	}

	sz := image.Bounds().Size()
	if sz.X > maxw {
		ve.Add(key, fmt.Errorf("%s width should be less than %d px, uploaded image width %d px", label, maxw, sz.X))
	}
	if sz.Y > maxh {
		ve.Add(key, fmt.Errorf("%s height should be less than %d px, uploaded image height %d px", label, maxh, sz.Y))
	}
}

func ValidateInt32Min(val int32, key, label string, ve ValidationErrors, min int32) {
	if val < min {
		ve.Add(key, fmt.Errorf("%s shouldn't be less than %d", label, min))
	}
}
