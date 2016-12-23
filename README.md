This web crawler is intended to be a fast and simple starting point for an incremental or continuous web crawler.

# Context
Web crawling is a problem that has seen a lot of attention in the late 90s and early 00s largely due to its vast economic potential.

Heydon et al.: Mercator: A scalable, extensible Web crawler
Cho et al. 1999: The Evolution of the Web and Implications for an Incremental Crawler
Brin et al.: The anatomy of a large-scale hypertextual web search engine

The consideration to design this crawler in a way that would support both incremental and continuous crawlers was derived from Cho et al. The way the different parts of the crawler hang together (downloading, analyzing, etc.) were taken from Brin/Page's paper.

# Running instructions
The program is written in golang, it can be run like so:
TODO

# Performance
- Page fetching
- Page analysis
- Sitemap storage

