package main

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"bytes"
	"io"
	"io/ioutil"
	"encoding/json"
)

func TestDownloadJob_Start_Valid(t *testing.T) {
	u := "https://www.google.com/"
	d := DownloadJob{Link:u}

	executeJob(&d)
	assert.Equal(t, 1, len(currentJobs.jobQueue)) // Ensure answer in channel

	j := <-currentJobs.jobQueue
	c := j.(*LinkExtractionJob)
	assert.Equal(t, u, c.Url)

	// Convert response to string.
	buf := new(bytes.Buffer)
	buf.ReadFrom(c.Data)
	s := buf.String()
	assert.Contains(t, s, "<html")
}

func TestDownloadJob_Start_Invalid(t *testing.T) {
	u := "google.com/"
	d := DownloadJob{Link:u}

	executeJob(&d)
	assert.Equal(t, 0, len(currentJobs.jobQueue))
}

func TestLinkExtractionJob_Start_None(t *testing.T) {
	h := "<html></html>"
	j := LinkExtractionJob{Url:"http://google.com/", Data:strToReadCloser(h)}

	executeJob(&j)

	assert.Equal(t, 0, len(currentJobs.jobQueue))
}

func TestLinkExtractionJob_Start_A(t *testing.T) {
	h := `<html><a href="link_destination"></a></html>`
	j := LinkExtractionJob{Url:"http://google.com/", Data:strToReadCloser(h)}

	s:= executeJob(&j)

	assert.Equal(t, 1, len(currentJobs.jobQueue))
	k := <- currentJobs.jobQueue
	q := k.(*QueueLinksJob)
	assert.Equal(t, "http://google.com/link_destination", q.Link)
	assert.Contains(t, s, `"Links":["http://google.com/link_destination"]`)
}

func TestLinkExtractionJob_Start_Link(t *testing.T) {
	h := `<html><link href="link_destination"></a></html>`
	j := LinkExtractionJob{Url:"http://google.com/", Data:strToReadCloser(h)}

	s := executeJob(&j)

	assert.Equal(t, 0, len(currentJobs.jobQueue))
	assert.Contains(t, s, `"Assets":["http://google.com/link_destination"]`)
}

func TestLinkExtractionJob_Start_Script(t *testing.T) {
	h := `<html><script src="link_destination"></a></html>`
	j := LinkExtractionJob{Url:"http://google.com/", Data:strToReadCloser(h)}

	s := executeJob(&j)

	assert.Equal(t, 0, len(currentJobs.jobQueue))
	assert.Contains(t, s, `"Assets":["http://google.com/link_destination"]`)
}

func TestLinkExtractionJob_Start_Img(t *testing.T) {
	h := `<html><img src="link_destination"></a></html>`
	j := LinkExtractionJob{Url:"http://google.com/", Data:strToReadCloser(h)}

	s := executeJob(&j)

	assert.Equal(t, 0, len(currentJobs.jobQueue))
	assert.Contains(t, s, `"Assets":["http://google.com/link_destination"]`)
}

func TestQueueLinksJob_Start_InDomain(t *testing.T) {
	visitedLinksLock <- true
	Domain = "google.com"
	l := "http://google.com/hi.jpg"

	j := QueueLinksJob{Link:l}
	executeJob(&j)

	assert.Equal(t, 1, len(currentJobs.jobQueue))
	k := <-currentJobs.jobQueue
	c := k.(*DownloadJob)
	assert.Equal(t, l, c.Link)
}

func TestQueueLinksJob_Start_OutOfDomain(t *testing.T) {
	visitedLinksLock <- true
	Domain = "google.com"
	l := "http://hello.com/hi.jpg"

	j := QueueLinksJob{Link:l}
	executeJob(&j)

	assert.Equal(t, 0, len(currentJobs.jobQueue))
}

// HELPERS.
// ========
func executeJob(job Job) string {
	outputLock <- true
	buf := bytes.NewBufferString("")
	output = json.NewEncoder(buf)
	currentJobs = NewJobQueue(10)
	currentJobs.Add(job)
	job.Start()
	<-currentJobs.jobQueue

	return buf.String() // Return the changes to the output file.
}

func strToReadCloser(s string) io.ReadCloser {
	return ioutil.NopCloser(bytes.NewReader([]byte(s)))
}