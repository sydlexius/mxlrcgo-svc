package queue

const (
	// PriorityScan is the baseline priority used for library scan-enqueued work.
	PriorityScan = 0
	// PriorityWebhook is used for webhook-enqueued work that should jump ahead of scan backlog.
	PriorityWebhook = 10
)
