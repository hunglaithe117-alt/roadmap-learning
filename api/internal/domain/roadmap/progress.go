package roadmap

import "math"

// Progress holds calculated roadmap completion metrics.
type Progress struct {
	TopicsTotal      int
	TopicsRequired   int
	TopicsDone       int
	TopicsInProgress int
	Percent          int
}

// ComputeProgress calculates progress metrics, excluding optional topics from the denominator.
func ComputeProgress(topics []Topic) Progress {
	p := Progress{}
	for _, t := range topics {
		if t.Deleted != 0 {
			continue
		}
		p.TopicsTotal++
		if t.IsOptional == Optional {
			continue
		}
		p.TopicsRequired++
		switch t.Status {
		case Done:
			p.TopicsDone++
		case InProgress:
			p.TopicsInProgress++
		}
	}
	p.Percent = percent(p.TopicsDone, p.TopicsRequired)
	return p
}

func percent(done, required int) int {
	if required <= 0 {
		return 0
	}
	return int(math.Floor(float64(done) * 100 / float64(required)))
}
