package app

import "github.com/sydlexius/mxlrcsvc-go/internal/models"

// InputsQueue is a FIFO queue for processing work items.
type InputsQueue struct {
	Queue []models.Inputs
}

// NewInputsQueue creates an empty InputsQueue.
func NewInputsQueue() *InputsQueue {
	return &InputsQueue{}
}

// Next returns the front item without removing it.
func (q *InputsQueue) Next() models.Inputs {
	return q.Queue[0]
}

// Pop removes and returns the front item.
func (q *InputsQueue) Pop() models.Inputs {
	tmp := q.Queue[0]
	q.Queue = q.Queue[1:]
	return tmp
}

// Push adds an item to the back of the queue.
func (q *InputsQueue) Push(i models.Inputs) {
	q.Queue = append(q.Queue, i)
}

// Len returns the number of items in the queue.
func (q *InputsQueue) Len() int {
	return len(q.Queue)
}

// Empty returns true if the queue has no items.
func (q *InputsQueue) Empty() bool {
	return len(q.Queue) == 0
}
