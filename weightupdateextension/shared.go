package weightupdateextension

import "sync"

var GlobalWeights = &SharedWeights{
	Weights:    make(map[string]float64),
	NumSources: 0,
}

type SharedWeights struct {
	sync.RWMutex
	Weights    map[string]float64
	NumSources int
}
