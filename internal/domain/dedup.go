package domain

// DedupCheckInput is the body for POST /dedup/check.
type DedupCheckInput struct {
	MD5s []string `json:"md5s"`
}

// DedupCheckResult is the response for POST /dedup/check.
type DedupCheckResult struct {
	Seen []string `json:"seen"`
}

// DedupURL is one entry for POST /dedup/mark.
type DedupURL struct {
	URL string `json:"url"`
	MD5 string `json:"md5"`
}

// DedupMarkInput is the body for POST /dedup/mark.
type DedupMarkInput struct {
	URLs []DedupURL `json:"urls"`
}

// DedupMarkResult is the response for POST /dedup/mark.
type DedupMarkResult struct {
	Count int `json:"count"`
}
