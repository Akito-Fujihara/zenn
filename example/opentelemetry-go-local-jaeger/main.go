package main

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"

	"otel-sample/database"
	"otel-sample/tracer"
)

func main() {
	// TracerProvider を初期化
	cleanup, err := tracer.InitTracer("otel-sample", "localhost:4317")
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	// データベース接続
	dsn := "root:password@tcp(localhost:3306)/otel_sample?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := database.NewDB(dsn)
	if err != nil {
		log.Fatal(err)
	}

	e := echo.New()

	// ミドルウェア
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(otelecho.Middleware("otel-sample"))

	// ルート
	e.GET("/users", func(c echo.Context) error {
		var users []database.User
		if err := db.WithContext(c.Request().Context()).Find(&users).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, users)
	})

	e.GET("/users/:id", func(c echo.Context) error {
		var user database.User
		if err := db.WithContext(c.Request().Context()).First(&user, c.Param("id")).Error; err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
		}
		return c.JSON(http.StatusOK, user)
	})

	e.POST("/users", func(c echo.Context) error {
		var user database.User
		if err := c.Bind(&user); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		if err := db.WithContext(c.Request().Context()).Create(&user).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusCreated, user)
	})

	e.Logger.Fatal(e.Start(":8080"))
}
