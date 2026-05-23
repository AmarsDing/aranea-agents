package kanban

import "os"

func lookupEnv(key string) string {
	return os.Getenv(key)
}
