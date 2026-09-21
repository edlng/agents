package approval

import "time"

func Sign(secret []byte, taskID, actor string, expires time.Time) (string, error) {
	return "", nil
}

func Verify(secret []byte, token, taskID string, now time.Time) (string, error) {
	return "", nil
}
