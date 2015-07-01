package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/qor/qor"
	"github.com/qor/qor/admin"
)

func main() {
	// setting up QOR admin
	Admin := admin.New(&qor.Config{DB: &db})
	// Admin := admin.New(&qor.Config{DB: Publish.DraftDB()})
	// Admin.AddResource(Publish)

	Admin.AddResource(
		&User{},
		&admin.Config{
			Menu: []string{"User Management"},
			Name: "Users",
		},
	)

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

	// alternate price display
	book.Meta(&admin.Meta{
		Name: "DisplayPrice",
		Valuer: func(value interface{}, context *qor.Context) interface{} {
			if value != nil {
				book := value.(*Book)
				return fmt.Sprintf("¥%v", book.Price)
			}
			return ""
		},
	})

	// defines the display field for authors in the product list
	book.Meta(&admin.Meta{
		Name:  "AuthorNames",
		Label: "Authors",
		Valuer: func(value interface{}, context *qor.Context) interface{} {
			if value == nil {
				return value
			}
			book := value.(*Book)
			if err := db.Model(&book).Related(&book.Authors, "Authors").Error; err != nil {
				panic(err)
			}

			log.Println(book.Authors)
			var authors string
			for i, author := range book.Authors {
				if i >= 1 {
					authors += ", "
				}
				authors += author.Name
			}
			return authors
		},
	})

	// what fields should be displayed in the books list on admin
	book.IndexAttrs("Title", "AuthorNames", "ReleaseDate", "DisplayPrice")
	// what fields should be editable in the book esit interface
	book.EditAttrs("Title", "Authors", "Synopsis", "ReleaseDate", "Price", "CoverImage")

	mux := http.NewServeMux()
	Admin.MountTo("/admin", mux)

	// frontend routes
	router := gin.Default()
	router.LoadHTMLGlob("templates/*")

	// serve static files
	router.StaticFS("/system/", http.Dir("public/system"))
	router.StaticFS("/assets/", http.Dir("public/assets"))

	// books - listing
	router.GET("/books", listBooksHandler)
	// single book - product page
	router.GET("/books/:id", viewBookHandler)

	mux.Handle("/", router)

	// handle login and logout of users
	Admin.SetAuth(&Auth{})
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/logout", logoutHandler)

	// start the server
	http.ListenAndServe(":9000", mux)
}
