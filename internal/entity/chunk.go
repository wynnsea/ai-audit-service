package entity

type Chunk struct {
	Index int
	Section string
	Page int
	Content string
	Embedding []float32
}