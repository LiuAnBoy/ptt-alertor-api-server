package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/google/gops/agent"
	"github.com/julienschmidt/httprouter"
	"github.com/robfig/cron/v3"

	"github.com/Ptt-Alertor/ptt-alertor/auth"
	"github.com/Ptt-Alertor/ptt-alertor/channels/telegram"
	ctrlr "github.com/Ptt-Alertor/ptt-alertor/controllers"
	"github.com/Ptt-Alertor/ptt-alertor/controllers/api"
	"github.com/Ptt-Alertor/ptt-alertor/jobs"
	"github.com/Ptt-Alertor/ptt-alertor/middleware"
)

var (
	telegramToken = os.Getenv("TELEGRAM_TOKEN")
)

type myRouter struct {
	httprouter.Router
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (mr myRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
	mr.Router.ServeHTTP(rw, r)
	log.WithFields(log.Fields{
		"method": r.Method,
		"IP":     r.RemoteAddr,
		"URI":    r.URL.Path,
		"status": rw.statusCode,
	}).Info("visit")
}

func newRouter() *myRouter {
	r := &myRouter{
		Router: *httprouter.New(),
	}
	return r
}

func main() {
	log.Info("Start Jobs")
	startJobs()

	router := newRouter()

	// health check
	router.GET("/hello", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"Hello World!"}`))
	})

	// boards apis
	router.GET("/boards/:boardName/articles/:code", ctrlr.BoardArticle)
	router.GET("/boards/:boardName/articles", ctrlr.BoardArticleIndex)
	router.GET("/boards", ctrlr.BoardIndex)

	// keyword apis
	router.GET("/keyword/boards", ctrlr.KeywordBoards)

	// author apis
	router.GET("/author/boards", ctrlr.AuthorBoards)

	// pushsum apis
	router.GET("/pushsum/boards", ctrlr.PushSumBoards)

	// articles apis
	router.GET("/articles", ctrlr.ArticleIndex)

	// telegram
	router.POST("/telegram/"+telegramToken, telegram.HandleRequest)

	// API v1 - Auth
	router.POST("/api/auth/register", api.Register)
	router.POST("/api/auth/login", api.Login)
	router.GET("/api/auth/me", auth.JWTAuth(api.Me))

	// API v1 - Notification bindings
	router.GET("/api/bindings", auth.JWTAuth(api.GetAllBindings))
	router.POST("/api/bindings/bind-code", auth.JWTAuth(api.GenerateBindCode))
	router.GET("/api/bindings/:service", auth.JWTAuth(api.BindingStatus))
	router.PATCH("/api/bindings/:service", auth.JWTAuth(api.SetBindingEnabled))
	router.DELETE("/api/bindings/:service", auth.JWTAuth(api.UnbindService))

	// API v1 - Telegram Web App binding
	router.POST("/api/webapp/telegram/confirm", auth.JWTAuth(api.TelegramWebAppConfirm))

	// API v1 - Subscriptions
	router.GET("/api/subscriptions", auth.JWTAuth(api.ListSubscriptions))
	router.POST("/api/subscriptions", auth.JWTAuth(api.CreateSubscription))
	router.GET("/api/subscriptions/:id", auth.JWTAuth(api.GetSubscription))
	router.PUT("/api/subscriptions/:id", auth.JWTAuth(api.UpdateSubscription))
	router.DELETE("/api/subscriptions/:id", auth.JWTAuth(api.DeleteSubscription))

	// API v1 - Stats (public)
	router.GET("/api/stats/subscriptions", api.ListSubscriptionStats)

	// API v1 - Admin
	router.POST("/api/admin/login", api.AdminLogin)
	router.GET("/api/admin/init", auth.RequireAdmin(api.AdminInit))
	router.GET("/api/admin/users", auth.RequireAdmin(api.AdminListUsers))
	router.GET("/api/admin/users/:id", auth.RequireAdmin(api.AdminGetUser))
	router.PUT("/api/admin/users/:id", auth.RequireAdmin(api.AdminUpdateUser))
	router.DELETE("/api/admin/users/:id", auth.RequireAdmin(api.AdminDeleteUser))
	router.POST("/api/admin/broadcast", auth.RequireAdmin(api.AdminBroadcast))

	// API v1 - Admin Roles
	router.GET("/api/admin/roles", auth.RequireAdmin(api.AdminListRoles))
	router.POST("/api/admin/roles", auth.RequireAdmin(api.AdminCreateRole))
	router.GET("/api/admin/roles/:role", auth.RequireAdmin(api.AdminGetRole))
	router.PUT("/api/admin/roles/:role", auth.RequireAdmin(api.AdminUpdateRole))
	router.DELETE("/api/admin/roles/:role", auth.RequireAdmin(api.AdminDeleteRole))

	// API v1 - PTT Account (VIP+ only)
	router.POST("/api/ptt-account", auth.JWTAuth(api.BindPTTAccount))
	router.DELETE("/api/ptt-account", auth.JWTAuth(api.UnbindPTTAccount))

	// gops agent
	if err := agent.Listen(agent.Options{Addr: ":6060", ShutdownCleanup: true}); err != nil {
		log.Fatal(err)
	}

	// Web Server
	log.Info("Web Server Start on Port 9090")
	srv := http.Server{
		Addr:    ":9090",
		Handler: middleware.CORS(router),
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal("ListenAndServer", err)
		}
	}()

	// graceful shutdown
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Info("Shutdown Web Server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.WithError(err).Fatal("Web Server Showdown Failed")
	}
	log.Info("Web Server Was Been Shutdown")
}

func startJobs() {
	go jobs.NewChecker().Run()
	go jobs.NewPushSumChecker().Run()
	go jobs.NewCommentChecker().Run()
	go jobs.NewPttMonitor().Run()
	c := cron.New()
	c.AddJob("@hourly", jobs.NewTop())
	c.AddJob("@hourly", jobs.NewCommentAggregator())
	c.AddJob("@every 48h", jobs.NewPushSumKeyReplacer())
	c.Start()
}

func init() {
	// for initial app
	// jobs.NewPushSumKeyReplacer().Run()
	// jobs.NewMigrateBoard(map[string]string{}).Run()
	// jobs.NewTop().Run()
	// jobs.NewCacheCleaner().Run()
	// jobs.NewGenerator().Run()
	// jobs.NewFetcher().Run()
	// jobs.NewMigrateDB().Run()
	// jobs.NewCategoryCleaner().Run()
}
