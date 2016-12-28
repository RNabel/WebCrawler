This web crawler is intended to be a fast and simple starting point for an incremental or continuous web crawler.

# Running instructions
The program is written in golang, it can be run like so:
```bash
go run crawler.go <start-page> <output-file-name?> <max-workers?> <max-jobs?>

e.g.
go run crawler.go http://tomblomfield.com/
```

- `start-page` is the starting point for the crawler, and defines which domain to limit crawling to. *Note:* `start-page` has to include a scheme e.g.: `http://tomblomfield.com`
- `output-file-name` (optional, default=`sitemap.txt`) the name of the file to save the results of the crawl to.
- `max-workers` (optional, default=10) The number of concurrent workers to use for the crawler.
- `max-jobs` (optional, default=10k) The maximum number of jobs to store until new job requests are ignored.

**IMPORTANT:** 
- If `max-workers` is >40 the program can crash due to to0 many open connections
- A low value (<1000) of `max-jobs` on a badly connected webpage can result in many parts of the page not being crawled 

# Output
## File output
The program saves stats about the crawled web pages as JSON-formatted objects, one per line, in the output file.
The JSON object has the following format:
```json
{
    "Link": "<crawled link>",
    "Links": ["<list of contained links>", ...],
    "Assets": ["<list of links to assets used on page>", ...]
}
```
When crawling `http://tomblomfield.com/` the following is the first line of the output file:
```json
{
    "Link":"http://tomblomfield.com/",
    "Links":["http://tomblomfield.com/post/97304410500/apple-is-propping-up-a-fundamentally-broken-payments","http://tomblomfield.com/post/61760552398/startup-series-part-1-interviewing-engineers",...],
    "Assets":["http://assets.tumblr.com/assets/scripts/pre_tumblelog.js","http://68.media.tumblr.com/b85a6d5d56c36b155d13ac7d2684d98a/tumblr_inline_n37sy2AZA91r5tr1m.png","http://68.media.tumblr.com/ddebec46b60f554989f09682fc3d8e71/tumblr_inline_mtj697fPI11r5tr1m.jpg",...]
}
```

## Terminal output
During execution, the crawler provides an overview of speed, progress etc.:
```
Crawling...
1754 / 2408 crawled. job queue length: 999, speed: 112.103494 pages/s
```

Format explained:
- first 2 numbers: `<pages crawled> / <links that have been scheduled for crawling>`
- job queue length is the number of items in the job queue, gives you an idea when jobs are dropped

# Performance
The crawler achieved **~70 pages/s** when tested on a Google Cloud Platform VM with 1.7GHz CPU, 2GB RAM, 140MBit download, and non-SSD disk.
 
Crawling tomblomfield.com, command:
```bash
time go run crawler.go http://tomblomfield.com/ tblomfield.txt 40 1000
```
results in: 
```
Crawling...
82 / 82 crawled. job queue length: 0, speed: 57.039791 pages/s  
Done!

real    0m2.935s
user    0m1.568s
sys     0m0.136s
```

Crawling ~130k wikipedia.org pages, command:
```bash
time go run crawler.go http://en.wikipedia.org/ wiki.txt 40 10000
```
results in: 
```
Crawling...
128312 / 161091 crawled. job queue length: 10000, speed: 70.214097 pages/s ^Csignal: interrupt

real	30m34.887s
user	26m30.364s
sys	1m47.748s
```
`wiki.txt` is about 2.4GB

# Overview of program structure

The program's structure was inspired by the following blog posts:

- Job/Worker/Dispatcher pattern adapted from [marcio.io - Handling 1 Million Requests per Minute with Go][5]
- HTML parsing adapted from [schier.co - A Simple Web Scraper in Go][4]

It breaks up the actions of the crawler into 3 separate jobs:
1. Download page (`DownloadJob`)
0. Parse page, and detect links and assets (`LinkExtractionJob`)
0. Filter links by domain and already-visited status (`QueueLinksJob`)

Each job is added to the `currentJobs` queue (of type `JobQueue`). The `Dispatcher` then assigns a 
`Worker` to each job.

The program terminates when the number of jobs added to `currentJobs` equals that of finished jobs 
(see end of each `Job.Start()` function).

## Things that could be improved
- Use probabalistic set rather than normal set (Bloom filter or HyperLogLog)
- Distribute file writes / use database for details
- Make program multi-machine

## Some academic inspiration
- Heydon et al. 1999: [Mercator: A scalable, extensible Web crawler][1]
- Cho et al. 1999: [The Evolution of the Web and Implications for an Incremental Crawler][2]
- Brin et al. 1998: [The anatomy of a large-scale hypertextual web search engine][3]

[1]: https://courses.cs.washington.edu/courses/cse454/15wi/papers/mercator.pdf
[2]: ilpubs.stanford.edu/376/1/1999-22.pdf
[3]: infolab.stanford.edu/~backrub/google.html
[4]: https://schier.co/blog/2015/04/26/a-simple-web-scraper-in-go.html
[5]: http://marcio.io/2015/07/handling-1-million-requests-per-minute-with-golang/