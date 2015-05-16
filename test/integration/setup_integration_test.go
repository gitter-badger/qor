package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"testing"

	. "github.com/onsi/gomega"
	"github.com/sclevine/agouti"
)

const (
	PORT = 9009
)

var (
	baseUrl = fmt.Sprintf("http://localhost:%v/admin", PORT)
	driver  *agouti.WebDriver
	page    *agouti.Page
)

func TestMain(m *testing.M) {
	var t *testing.T
	var err error

	// command := []string{"java", "-jar", "selenium-server-standalone-2.44.0.jar", "-port", "9090"}
	// driver = agouti.NewWebDriver("http://localhost:9090/wd/hub", command)
	driver = agouti.ChromeDriver()
	driverErr := driver.Start()
	if driverErr != nil {
		panic(driverErr)
	}

	go Start(PORT)

	page, err = driver.NewPage(agouti.Browser("chrome"))
	if err != nil {
		panic(err)
	}

	RegisterTestingT(t)
	test := m.Run()

	driver.Stop()
	os.Exit(test)
}

func StopDriverOnPanic() {
	var t *testing.T
	if r := recover(); r != nil {
		debug.PrintStack()
		fmt.Println("Recovered in f", r)
		driver.Stop()
		t.Fail()
	}
}
