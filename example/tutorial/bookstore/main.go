package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qor/qor"
	"github.com/qor/qor/admin"
)

func main() {
	// setting up QOR admin
	Admin := admin.New(&qor.Config{DB: &db})
	Admin.AddResource(
		&Author{},
		&admin.Config{Menu: []string{
			"Author Management"},
			Name: "Author",
		},
	)

	book := Admin.AddResource(
		&Book{},
		&admin.Config{
			Menu: []string{"Book Management"},
			Name: "Books",
		},
	)

	book.Meta(&admin.Meta{
		Name: "Price",
		Valuer: func(value interface{}, context *qor.Context) interface{} {
			book := value.(*Book)
			return fmt.Sprintf("¥%v", book.Price)
		},
	})

	book.Meta(&admin.Meta{
		Name: "AuthorNames",
		Valuer: func(value interface{}, context *qor.Context) interface{} {
			book := value.(*Book)
			if err := db.Model(&book).Related(&book.Authors, "Authors").Error; err != nil {
				panic(err)
			}

			log.Println(book.Authors)
			var authors string
			for i, author := range book.Authors {
				log.Println("author.Name", author.Name)
				if i >= 1 {
					authors += ", "
				}
				authors += author.Name
			}
			return authors
		},
	})

	// what fields should be displayed in the books list on admin
	book.IndexAttrs("Title", "AuthorNames", "ReleaseDate", "Price")
	book.EditAttrs("Title", "Authors", "ReleaseDate", "Price")

	// defines the edit field for authors of the book
	book.Meta(&admin.Meta{Name: "Authors", Type: "select_many"})

	// step 5
	Admin.AddResource(
		&User{},
		&admin.Config{
			Menu: []string{"User Management"},
			Name: "Users",
		},
	)

	mux := http.NewServeMux()
	Admin.MountTo("/admin", mux)

	// Chapter 3: serve static files
	mux.Handle(
		"/system/",
		http.FileServer(http.Dir("public")),
	)
	mux.Handle(
		"/assets/",
		http.FileServer(http.Dir("public")),
	)

	// frontend routes
	router := gin.Default()
	router.LoadHTMLGlob("templates/*")

	// all books - listing
	router.GET("/books", func(ctx *gin.Context) {
		var books []*Book

		if err := db.Find(&books).Error; err != nil {
			panic(err)
		}

		ctx.HTML(
			http.StatusOK,
			"list.tmpl",
			gin.H{
				"title": "List of Books",
				"books": books,
			},
		)
	})

	// single book - product page
	router.GET("/book/:id", func(ctx *gin.Context) {
		id, err := strconv.ParseUint(ctx.Params.ByName("id"), 10, 64)
		if err != nil {
			panic(err)
		}
		var book = &Book{}
		if err := db.Find(&book, id).Error; err != nil {
			panic(err)
		}

		if err := db.Model(&book).Related(&book.Authors, "Authors").Error; err != nil {
			panic(err)
		}

		ctx.HTML(
			http.StatusOK,
			"book.tmpl",
			gin.H{
				"book": book,
			},
		)
	})
	mux.Handle("/", router)

	// handle login and logout of users
	Admin.SetAuth(&Auth{})

	mux.HandleFunc("/login", func(writer http.ResponseWriter, request *http.Request) {
		var user User

		if request.Method == "POST" {
			request.ParseForm()
			if !db.First(&user, "name = ?", request.Form.Get("username")).RecordNotFound() {
				cookie := http.Cookie{Name: "userid", Value: fmt.Sprintf("%v", user.ID), Expires: time.Now().AddDate(1, 0, 0)}
				http.SetCookie(writer, &cookie)
				writer.Write([]byte("<html><body>logged as `" + user.Name + "`, go <a href='/admin'>admin</a></body></html>"))
			} else {
				http.Redirect(writer, request, "/login?failed_to_login", 301)
			}
		} else if userid, err := request.Cookie("userid"); err == nil {
			if !db.First(&user, "id = ?", userid.Value).RecordNotFound() {
				writer.Write([]byte("<html><body>already logged as `" + user.Name + "`, go <a href='/admin'>admin</a></body></html>"))
			} else {
				http.Redirect(writer, request, "/logout", http.StatusSeeOther)
			}
		} else {
			writer.Write([]byte(`<html><form action="/login" method="POST"><input name="username" value="" placeholder="username"><input type=submit value="Login"></form></html>`))
		}
	})

	mux.HandleFunc("/logout", func(writer http.ResponseWriter, request *http.Request) {
		cookie := http.Cookie{Name: "userid", MaxAge: -1}
		http.SetCookie(writer, &cookie)
		http.Redirect(writer, request, "/login?logged_out", http.StatusSeeOther)
	})

	http.ListenAndServe(":9000", mux)
}
