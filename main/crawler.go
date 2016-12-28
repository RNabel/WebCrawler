// TODO add header.
// Job/Worker/Dispatcher pattern adapted from:
// 	http://marcio.io/2015/07/handling-1-million-requests-per-minute-with-golang/
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
)

// TYPE DEFINITIONS.
// Create a simple string set. Adapted from:
// 	http://softwareengineering.stackexchange.com/questions/177428/sets-data-structure-in-golang
type StringSet struct {
	set map[string]bool
}

type ResponseData struct {
	data io.Reader
	url  string
}

// GLOBAL VARS.
// ============
var (
	// Set up visited sites map and lock.
	VisitedLinks = StringSet{set:make(map[string]bool)}
	VisitedLinksLock = make(chan bool, 1)
	MaxWorkers = 10
	MaxJobs = 10000
	JobQueue = make(chan Job, MaxJobs)
	Out *os.File
	JobGroup sync.WaitGroup
)

// JOB DEFINITIONS.
// ================
type Job interface {
	Start()
}

type DownloadJob struct {
	link     string
	jobQueue chan Job
}

type LinkExtractionJob struct {
	contents ResponseData
	jobQueue chan Job
}

type QueueLinksJob struct {
	link     string
	jobQueue chan Job
}

func (j *DownloadJob) Start() {
	// Get next link from link channel.
	link := j.link

	// Download page and add output to outChan.
	//fmt.Printf("Downloading: %s\n", link)
	response, err := http.Get(link)
	if err == nil {
		JobGroup.Add(1)
		JobQueue <- &LinkExtractionJob{
			ResponseData{data:response.Body, url:link},
			JobQueue,
		}
	}
	fmt.Println("Finished DownloadJob, link: " + link)
	JobGroup.Done()
}

// Adapted from https://schier.co/blog/2015/04/26/a-simple-web-scraper-in-go.html
func (j *LinkExtractionJob) Start() {
	rd := j.contents
	link := rd.url

	tokenizer := html.NewTokenizer(rd.data)
	cont := true
	for cont {
		nextToken := tokenizer.Next()
		switch {
		case nextToken == html.ErrorToken:
			// Finished with document.
			//fmt.Println("Done with the document.")
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
					//fmt.Println("Sending " + href + " to be queued.")
					JobGroup.Add(1)
					JobQueue <- &QueueLinksJob{link:href}
					//fmt.Printf("%s queued, q length: %d\n", href, len(JobQueue))
				}
			}
		}
	}
	fmt.Println("Finished LinkExtractionJob: " + rd.url)
	JobGroup.Done()
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

func (j *QueueLinksJob) Start() {
	//fmt.Println("Started QueueLinksJob")
	<-VisitedLinksLock // Acquire lock.

	//fmt.Println("queueLinks acquired lock")
	_, found := VisitedLinks.set[j.link]
	if !found && strings.HasPrefix(j.link, "http://tomblomfield.com/") {
		// Add link to visitedLinks set.
		VisitedLinks.set[j.link] = true
		Out.WriteString(j.link + "\n") // Add the current link to the output file.
		// Crawl link.
		//fmt.Printf("Before Added: %s, queue length: %d \n", j.link, len(JobQueue))
		//fmt.Printf("QueueLinksJob, queue size: %d", len(JobQueue))
		JobGroup.Add(1)
		JobQueue <- &DownloadJob{link:j.link, jobQueue:JobQueue} // Create new download job.
	} else {
		//fmt.Println("Rejected: " + currentLink)
	}

	VisitedLinksLock <- true // Release lock.
	fmt.Println("Finished QueueLinksJob")
	JobGroup.Done()
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
	jobQueue   chan Job
	workerPool chan chan Job
	numWorkers int
}

func NewDispatcher(maxWorkers int, jobQueue chan Job) *Dispatcher {
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
		case job := <-d.jobQueue: // New job received.
		// Wait for idle worker and process request.
		// Worker registeres its job channel when it becomes idle.
			jobChannel := <-d.workerPool
			jobChannel <- job // Assign job to the worker's job channel.
		}
	}
}

func main() {
	// Ensure starting web page passed.
	if len(os.Args) < 2 {
		fmt.Printf("usage: %s <root page>", os.Args[0])
		os.Exit(1)
	}

	// Set up and start dispatcher.
	//jobQueue := make(chan Job, MaxJobs)
	disp := NewDispatcher(MaxWorkers, JobQueue)
	disp.Start()

	// Set up output file.
	var err error
	Out, err = os.Create("sitemap.txt")
	if err != nil {
		fmt.Println("Can't write sitemap.txt...")
		os.Exit(1)
	}

	// Make link set lock available.
	VisitedLinksLock <- true

	// Push start page into the downloadLinks channel.
	startPage := os.Args[1]
	fmt.Println("Start page set: " + startPage)
	JobGroup.Add(1)
	JobQueue <- &DownloadJob{jobQueue:JobQueue, link: startPage}

	// Detect when all goroutines are done.
	// No more jobs in the queue AND no worker active AND disp waiting for jobs.
	JobGroup.Wait()
	fmt.Println("All finished!")
}