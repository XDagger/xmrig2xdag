package utils

import "sync"

// https://github.com/zeromicro/go-zero/blob/master/core/collection/safemap.go
const (
	copyThreshold = 1000
	maxDeletion   = 10000
)

// SafeMap provides a map alternative to avoid memory leak.
// This implementation is not needed until issue below fixed.
// https://github.com/golang/go/issues/20135
type SafeMap struct {
	lock        sync.RWMutex
	deletionOld int
	deletionNew int
	dirtyOld    map[uint64]interface{}
	dirtyNew    map[uint64]interface{}
}

// NewSafeMap returns a SafeMap.
func NewSafeMap() *SafeMap {
	return &SafeMap{
		dirtyOld: make(map[uint64]interface{}),
		dirtyNew: make(map[uint64]interface{}),
	}
}

// Del deletes the value with the given key from m.
func (m *SafeMap) Del(key uint64) {
	m.lock.Lock()
	if _, ok := m.dirtyOld[key]; ok {
		m.dirtyOld[key] = nil
		delete(m.dirtyOld, key)
		m.deletionOld++
	} else if _, ok := m.dirtyNew[key]; ok {
		m.dirtyNew[key] = nil
		delete(m.dirtyNew, key)
		m.deletionNew++
	}
	if m.deletionOld >= maxDeletion && len(m.dirtyOld) < copyThreshold {
		for k, v := range m.dirtyOld {
			m.dirtyNew[k] = v
		}
		m.dirtyOld = m.dirtyNew
		m.deletionOld = m.deletionNew
		m.dirtyNew = make(map[uint64]interface{})
		m.deletionNew = 0
	}
	if m.deletionNew >= maxDeletion && len(m.dirtyNew) < copyThreshold {
		for k, v := range m.dirtyNew {
			m.dirtyOld[k] = v
		}
		m.dirtyNew = make(map[uint64]interface{})
		m.deletionNew = 0
	}
	m.lock.Unlock()
}

// Get gets the value with the given key from m.
func (m *SafeMap) Get(key uint64) (interface{}, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if val, ok := m.dirtyOld[key]; ok {
		return val, true
	}

	val, ok := m.dirtyNew[key]
	return val, ok
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
func (m *SafeMap) Range(f func(key, val interface{}) bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	for k, v := range m.dirtyOld {
		if !f(k, v) {
			return
		}
	}
	for k, v := range m.dirtyNew {
		if !f(k, v) {
			return
		}
	}
}

// Set sets the value into m with the given key.
func (m *SafeMap) Set(key uint64, value interface{}) {
	m.lock.Lock()
	if m.deletionOld <= maxDeletion {
		if _, ok := m.dirtyNew[key]; ok {
			m.dirtyNew[key] = nil
			delete(m.dirtyNew, key)
			m.deletionNew++
		}
		m.dirtyOld[key] = value
	} else {
		if _, ok := m.dirtyOld[key]; ok {
			m.dirtyOld[key] = nil
			delete(m.dirtyOld, key)
			m.deletionOld++
		}
		m.dirtyNew[key] = value
	}
	m.lock.Unlock()
}

// Size returns the size of m.
func (m *SafeMap) Size() int {
	m.lock.RLock()
	size := len(m.dirtyOld) + len(m.dirtyNew)
	m.lock.RUnlock()
	return size
}
