// Simple Web crawler written in golang, see README for running instructions.
// Job/Worker/Dispatcher pattern adapted from:
// 	http://marcio.io/2015/07/handling-1-million-requests-per-minute-with-golang/
// Web site parsing adapted from:
// 	https://schier.co/blog/2015/04/26/a-simple-web-scraper-in-go.html
package main

import (
	"fmt"
	"os"
	"net/http"
	"golang.org/x/net/html"
	"strings"
	"net/url"
	"io"
	"sync"
	"strconv"
	"encoding/json"
)

// JobQueue wraps a channel with a sync.WaitGroup, which allows the main thread to wait until all
// 	jobs have been processed, even if total numer of jobs is not known.
type JobQueue struct {
	jobGroup sync.WaitGroup
	jobQueue chan Job
}

func NewJobQueue(maxJobs int) JobQueue {
	jq := make(chan Job, maxJobs)
	return JobQueue{jobQueue: jq}
}

func (jq *JobQueue) Add(job Job) {
	jq.jobGroup.Add(1)
	jq.jobQueue <- job
}

func (jq *JobQueue) Done() {
	jq.jobGroup.Done()
}

func (jq *JobQueue) Wait() {
	jq.jobGroup.Wait()
}

// GLOBAL VARS.
// ============
var (
	// Set up visited sites map and lock.
	visitedLinks = make(map[string]bool)
	visitedLinksLock = make(chan bool, 1)

	// Setting defaults.
	MaxWorkers = 10
	MaxJobs = 10000
	OutpufFileName = "sitemap.txt"

	currentJobs = NewJobQueue(MaxJobs)

	output *json.Encoder

	Domain string
	crawledPages = 1
	totalPages = 1
)

// JOB DEFINITIONS.
// ================
type Job interface {
	Start()
}

type DownloadJob struct {
	link string
}

type LinkExtractionJob struct {
	data io.Reader
	url  string
}

type QueueLinksJob struct {
	link string
}

func (j *DownloadJob) Start() {
	// Download page and add output to outChan.
	//fmt.Printf("Downloading: %s\n", link)
	response, err := http.Get(j.link)
	if err == nil {
		currentJobs.Add(&LinkExtractionJob{data:response.Body, url:j.link})
	}
	currentJobs.Done()
}

func (j *LinkExtractionJob) Start() {
	tokenizer := html.NewTokenizer(j.data)
	cont := true

	links := make(map[string]bool)
	assets := make(map[string]bool)

	for cont {
		nextToken := tokenizer.Next()
		switch {
		case nextToken == html.ErrorToken:
			// Finished with document.
			cont = false
		case nextToken == html.StartTagToken:
			token := tokenizer.Token()
			var link string
			isLink := false

			switch token.Data {
			case "a":
				link = getAttr(token, "href")
				isLink = true
			case "link":
				link = getAttr(token, "href")
			case "script":
				link = getAttr(token, "src")
			case "img":
				link = getAttr(token, "src")
			default:
				continue
			}

			if link != "" {
				link = processLink(link, j.url)

				if isLink {
					currentJobs.Add(&QueueLinksJob{link:link})
					links[link] = true
				} else {
					assets[link] = true
				}
			}
		}
	}

	// Add all details to the output file.
	writeDetails(j.url, links, assets)

	currentJobs.Done()
}

// Helper for LinkExtractionJob.
func getAttr(token html.Token, name string) string {
	for _, a := range token.Attr {
		if a.Key == name {
			return a.Val
		}
	}

	return ""
}

func processLink(href string, context string) string {
	uMain, _ := url.Parse(href)
	if !strings.HasPrefix(href, "http") {
		// Check if local path.
		uBase, _ := url.Parse(context)
		uMain = uBase.ResolveReference(uMain)
	}

	// Strip fragments.
	uMain.Fragment = ""

	return uMain.String()
}

func (j *QueueLinksJob) Start() {
	<-visitedLinksLock // Acquire lock.

	_, found := visitedLinks[j.link]

	u, _ := url.Parse(j.link)

	if !found && u.Host == Domain {
		// Add link to visitedLinks set.
		visitedLinks[j.link] = true
		totalPages++ // Increase counter for total number of pages to be crawled.

		currentJobs.Add(&DownloadJob{link:j.link}) // Create new download job.
	}

	visitedLinksLock <- true // Release lock.
	currentJobs.Done()
}

type PageRecord struct {
	Link string
	Links []string
	Assets []string
}

func writeDetails(link string, links map[string]bool, assets map[string]bool) {
	l := setToArr(links)
	a := setToArr(assets)

	r := PageRecord{Link: link, Links: l, Assets:a}
	output.Encode(r)

	fmt.Printf("\r%d / %d crawled. ", crawledPages, totalPages)
	crawledPages++
}

func setToArr(set map[string]bool) []string {
	l := make([]string, len(set))
	i := 0
	for el, _ := range set {
		l[i] = el
		i++
	}
	return l
}

// WORKER.
// =======
type Worker struct {
	pool   chan chan Job
	myJobs chan Job
	quit   chan bool
}

func NewWorker(workerPool chan chan Job) Worker {
	// Constructor for a worker.
	return Worker{
		pool: workerPool,
		myJobs: make(chan Job),
		quit: make(chan bool),
	}
}

func (w *Worker) Start() {
	go func() {
		for {
			// Register the worker as available.
			w.pool <- w.myJobs

			// Wait for job or quit command.
			select {
			case newJob := <-w.myJobs:
			// Received job.
				newJob.Start()
			case <-w.quit:
			// Received stop instruction.
				return
			}
		}
	}()
}

func (w *Worker) Stop() {
	go func() {
		w.quit <- true
	}()
}

// DISPATCHER.
// ===========
type Dispatcher struct {
	jobQueue   JobQueue
	workerPool chan chan Job
	numWorkers int
}

func NewDispatcher(maxWorkers int, jobQueue JobQueue) *Dispatcher {
	// Dispatcher constructor.
	// Create new worker pool.
	pool := make(chan chan Job, maxWorkers)
	return &Dispatcher{
		workerPool: pool,
		numWorkers: maxWorkers,
		jobQueue: jobQueue,
	}
}

func (d *Dispatcher) Start() {
	// Start the specified number of workers.
	for i := 1; i <= d.numWorkers; i++ {
		newWorker := NewWorker(d.workerPool)
		newWorker.Start()
	}

	// Start dispatching jobs.
	go d.dispatch()
}

func (d *Dispatcher) dispatch() {
	for {
		select {
		case job := <-d.jobQueue.jobQueue:
		// New job received.
		// Wait for idle worker and process request.
		// Worker registeres its job channel when it becomes idle.
			jobChannel := <-d.workerPool
			jobChannel <- job // Assign job to the worker's job channel.
		}
	}
}

func main() {
	// Set all possible program argument to default values.
	var startPage string
	ofn := OutpufFileName
	mw := MaxWorkers

	// Read in program arguments and catch possible errors.
	switch {
	case len(os.Args) == 1:
		fmt.Printf("usage: %s <root page> <outputfilename?> <maxworkers?>", os.Args[0])
		os.Exit(1)
	case len(os.Args) > 3:
		mw, _ = strconv.Atoi(os.Args[3])
		fallthrough
	case len(os.Args) > 2:
		ofn = os.Args[2]
		fallthrough
	case len(os.Args) > 1:
		startPage = os.Args[1]
	}

	u, _ := url.Parse(startPage)
	Domain = u.Host

	// Set up and start dispatcher.
	d := NewDispatcher(mw, currentJobs)
	d.Start()

	// Set up output file.
	o, e := os.Create(ofn)
	output = json.NewEncoder(o)
	if e != nil {
		fmt.Printf("Can't write %s...\n", ofn)
		os.Exit(1)
	}

	// Make link set lock available.
	visitedLinksLock <- true
	fmt.Println("Crawling...")
	// Push start page into the downloadLinks channel.
	currentJobs.Add(&DownloadJob{link: startPage})

	// Detect when all goroutines are done, i.e. all jobs have been processed.
	currentJobs.Wait()
	fmt.Println("Done!")
}