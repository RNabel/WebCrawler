package main

import (
	"fmt"
	"os"
	"net/http"
	"golang.org/x/net/html"
	"time"
	"strings"
	"net/url"
)

// Create a simple string set. Adapted from:
// 	http://softwareengineering.stackexchange.com/questions/177428/sets-data-structure-in-golang
type StringSet struct {
	set map[string]bool
}

type ResponseTuple struct { // FIXME Use tuple instead.
	resp http.Response
	url  string
}
// TODO Evaluate whether to factor out state into a struct.

func download(linkChan chan string, outChan chan ResponseTuple) {
	fmt.Println("Started downloader.")
	for {
		// Get next link from link channel.
		link := <-linkChan

		// Download page and add output to outChan.
		fmt.Printf("Downloading: %s\n", link)
		response, err := http.Get(link)
		if err == nil {
			outChan <- ResponseTuple{resp:*response, url:link}
		}
	}
}

// Adapted from https://schier.co/blog/2015/04/26/a-simple-web-scraper-in-go.html
func extractLinks(contents chan ResponseTuple, toBeFiltered chan string) {
	for {
		fmt.Println("Extractor waiting for content...")
		tup := <-contents
		link := tup.url

		tokenizer := html.NewTokenizer(tup.resp.Body)
		fmt.Println("Got new page content :)")
		cont := true

		for cont {
			nextToken := tokenizer.Next()
			switch {
			case nextToken == html.ErrorToken:
				// Finished with document.
				fmt.Println("Done with the document.")
				cont = false
			case nextToken == html.StartTagToken:
				token := tokenizer.Token()
				isAnchor := token.Data == "a"
				if isAnchor {
					href := handleAnchorToken(token)
					if href != "" {
						if !strings.HasPrefix(href, "http") {
							href = handleLocalPaths(href, link)
						}
						// Check whether anchor had anchor href.
						fmt.Println("Sending " + href + " to be queued.")
						toBeFiltered <- href
						fmt.Println(href + " queued.")
					}
				}
			case nextToken == html.EndTagToken:
				token := tokenizer.Token()
				if token.Data == "html" {
					fmt.Println("Found closing html token!!!")
					cont = false
				}
			}
		}
	}
}

// Helper for extractLinks.
func handleAnchorToken(token html.Token) string {
	for _, a := range token.Attr {
		if a.Key == "href" {
			//fmt.Printf("Found HREF: %s\n", a.Val)
			return a.Val
		}
	}

	return ""
}

func handleLocalPaths(href string, context string) string {
	u, _ := url.Parse(context)
	u_relative, _ := url.Parse(href)
	u = u.ResolveReference(u_relative)

	return u.String()
}

func queueLinks(linksToBeFiltered chan string, linksToDownload chan string, visitedPages StringSet, visitedPagesLock chan bool) {
	for {
		// Get link and check whether the link is already contained in the set.
		currentLink := <-linksToBeFiltered
		//fmt.Println("queueLinks: " + currentLink)

		<-visitedPagesLock  // Acquire lock.
		//fmt.Println("queueLinks acquired lock")
		_, found := visitedPages.set[currentLink]
		if !found && strings.HasPrefix(currentLink, "http://tomblomfield.com/") {
			// Add link to visitedPages set.
			visitedPages.set[currentLink] = true
			// Crawl link.
			fmt.Println("Before Added: " + currentLink)
			linksToDownload <- currentLink
			fmt.Println("Added: " + currentLink)
		} else {
			//fmt.Println("Rejected: " + currentLink)
		}

		visitedPagesLock <- true // Release lock.

	}
}

func main() {
	// Ensure starting web page passed.
	if len(os.Args) < 2 {
		fmt.Printf("usage: %s <root page>", os.Args[0])
		os.Exit(1)
	}


	// Set up required channels exist, each to be consumed by a goroutine:
	// 	1. (downloadLinks) Channel of links to be downloaded,
	// 	2. (pageContents) Channel of page io to be analyzed for URLs,
	// 	3. (linksToBeFiltered) Channel with links from downloaded page, which are filtered
	// 		to prevent repeated downloads.
	downloadLinks := make(chan string, 10)
	pageContents := make(chan ResponseTuple, 10)
	linksToBeFiltered := make(chan string, 10)

	// Set up visited sites map and lock.
	visitedLinks := StringSet{set:make(map[string]bool)}
	visitedLinksLock := make(chan bool, 1)
	visitedLinksLock <- true // Initialize lock as available.


	// Push start page into the downloadLinks channel.
	startPage := os.Args[1]
	downloadLinks <- startPage
	fmt.Println("Start page set: " + startPage)

	go download(downloadLinks, pageContents)

	go extractLinks(pageContents, linksToBeFiltered)

	go queueLinks(linksToBeFiltered, downloadLinks, visitedLinks, visitedLinksLock)

	time.Sleep(time.Minute * 5)
	fmt.Println("Finished.")
}