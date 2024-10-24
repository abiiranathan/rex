package main

import (
	"log"
	"net/http"

	"github.com/abiiranathan/rex/rex"
)

func main() {
	router := rex.NewRouter()

	router.GET("/", func(ctx *rex.Context) error {
		return ctx.String("Hello, World!")
	})

	router.GET("/json", func(ctx *rex.Context) error {
		return ctx.JSON(rex.Map{"message": "Hello, World!"})
	})

	router.GET("/json/{username}", func(ctx *rex.Context) error {
		return ctx.String("Hello %s", ctx.Param("username"))
	})

	router.GET("/xml", func(ctx *rex.Context) error {
		type Message struct {
			Payload string `xml:"payload"`
		}
		return ctx.XML(Message{"Hello, World!"})
	})

	log.Fatal(http.ListenAndServe(":8080", router))

}
