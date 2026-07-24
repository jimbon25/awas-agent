package sessionlock

import "sync"


var (
	fileLocks = make(map[string]*sync.RWMutex)
	locksMu   sync.Mutex
)

func GetLock(sessionID string) *sync.RWMutex {
	locksMu.Lock()
	defer locksMu.Unlock()

	if lock, ok := fileLocks[sessionID]; ok {
		return lock
	}
	lock := &sync.RWMutex{}
	fileLocks[sessionID] = lock
	return lock
}

func LockWrite(sessionID string) {
	lock := GetLock(sessionID)
	lock.Lock()
}

func UnlockWrite(sessionID string) {
	lock := GetLock(sessionID)
	lock.Unlock()
}

func LockRead(sessionID string) {
	lock := GetLock(sessionID)
	lock.RLock()
}

func UnlockRead(sessionID string) {
	lock := GetLock(sessionID)
	lock.RUnlock()
}

func RemoveLock(sessionID string) {
	locksMu.Lock()
	defer locksMu.Unlock()
	delete(fileLocks, sessionID)
}
