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
	VisitedLinks = make(map[string]bool)
	VisitedLinksLock = make(chan bool, 1)

	// Setting defaults.
	MaxWorkers = 10
	MaxJobs = 10000
	OutpufFileName = "sitemap.txt"

	CurrentJobs = NewJobQueue(MaxJobs)
	Out *os.File
)

// JOB DEFINITIONS.
// ================
type Job interface {
	Start()
}

type DownloadJob struct {
	link     string
}

type LinkExtractionJob struct {
	data io.Reader
	url  string
}

type QueueLinksJob struct {
	link     string
}

func (j *DownloadJob) Start() {
	// Download page and add output to outChan.
	//fmt.Printf("Downloading: %s\n", link)
	response, err := http.Get(j.link)
	if err == nil {
		CurrentJobs.Add(&LinkExtractionJob{data:response.Body, url:j.link})
	}
	CurrentJobs.Done()
}

func (j *LinkExtractionJob) Start() {
	tokenizer := html.NewTokenizer(j.data)
	cont := true
	for cont {
		nextToken := tokenizer.Next()
		switch {
		case nextToken == html.ErrorToken:
			// Finished with document.
			cont = false
		case nextToken == html.StartTagToken:
			token := tokenizer.Token()
			isAnchor := token.Data == "a"
			if isAnchor {
				href := handleAnchorToken(token)
				if href != "" {
					if !strings.HasPrefix(href, "http") {
						href = handleLocalPaths(href, j.url)
					}
					// Check whether anchor had anchor href.
					CurrentJobs.Add(&QueueLinksJob{link:href})
				}
			}
		}
	}
	CurrentJobs.Done()
}

// Helper for LinkExtractionJob.
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

func (j *QueueLinksJob) Start() {
	<-VisitedLinksLock // Acquire lock.

	_, found := VisitedLinks[j.link]
	if !found && strings.HasPrefix(j.link, "http://tomblomfield.com/") {
		// Add link to visitedLinks set.
		VisitedLinks[j.link] = true
		Out.WriteString(j.link + "\n") // Add the current link to the output file.
		// Crawl link.
		//fmt.Printf("Before Added: %s, queue length: %d \n", j.link, len(JobQueue))
		//fmt.Printf("QueueLinksJob, queue size: %d", len(JobQueue))
		CurrentJobs.Add(&DownloadJob{link:j.link}) // Create new download job.
	}

	VisitedLinksLock <- true // Release lock.
	CurrentJobs.Done()
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
		case job := <-d.jobQueue.jobQueue: // New job received.
		// Wait for idle worker and process request.
		// Worker registeres its job channel when it becomes idle.
			jobChannel := <-d.workerPool
			jobChannel <- job // Assign job to the worker's job channel.
		}
	}
}

func main() {
	// Set all possible program argument to default values.
	startPage := ""
	ofn := OutpufFileName
	mw := MaxWorkers

	// Read in program arguments and catch possible errors.
	switch {
	case len(os.Args) == 1:
		fmt.Printf("usage: %s <root page> <outputfilename?> <maxworkers?>", os.Args[0])
		os.Exit(1)
	case len(os.Args) > 1:
		startPage = os.Args[1]
	case len(os.Args) > 2:
		ofn = os.Args[2]
	case len(os.Args) > 3:
		mw, _ = strconv.Atoi(os.Args[3])
	}

	// Set up and start dispatcher.
	d := NewDispatcher(mw, CurrentJobs)
	d.Start()

	// Set up output file.
	var e error
	Out, e = os.Create(ofn)
	if e != nil {
		fmt.Printf("Can't write %s...\n", ofn)
		os.Exit(1)
	}

	// Make link set lock available.
	VisitedLinksLock <- true
	fmt.Println("Crawling...")
	// Push start page into the downloadLinks channel.
	CurrentJobs.Add(&DownloadJob{link: startPage})

	// Detect when all goroutines are done, i.e. all jobs have been processed.
	CurrentJobs.Wait()
	fmt.Println("All finished!")
}