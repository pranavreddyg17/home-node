package transfer

import "time"

// Whole-object checks may read a cold 1 GiB object on a disk capped at
// 50 MiB/s: I/O alone can exceed the ordinary 20-second request budget.
// Leave response margin below the controller's 90-second header deadline.
const ServerWriteTimeout = 90 * time.Second

func requestBudget(operation string) time.Duration {
	if operation == "finalize" || operation == "stat" {
		return 85 * time.Second
	}
	return 20 * time.Second
}
