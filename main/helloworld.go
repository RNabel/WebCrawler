package main

import (
	"fmt"
	"net/http"
	"golang.org/x/net/html"
	"io"
)

func getPageStream(path string) io.Reader {
	fmt.Println("Requesting page.")
	resp, _ := http.Get(path)

	return resp.Body
}

func extractURLs(stream io.Reader) []string {

}

func main() {
	fmt.Println(getPageStream("https://www.google.com"))
}

