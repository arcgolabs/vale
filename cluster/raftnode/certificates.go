package raftnode

import (
	"bytes"
	"encoding/json"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	sm "github.com/lni/dragonboat/v3/statemachine"
	"github.com/samber/oops"
)

func applyCertificateStore(next State, record *CertificateRecord) (State, sm.Result, error) {
	if record == nil || strings.TrimSpace(record.Key) == "" {
		return next, sm.Result{}, oops.
			In("raftnode").
			New("certificate store command requires key")
	}
	recordCopy := cloneCertificateRecord(*record)
	next.Certificates.RemoveIf(func(existing CertificateRecord) bool {
		return existing.Key == recordCopy.Key
	})
	next.Certificates.Add(recordCopy)
	return next, commandResult(next.Version, true, ""), nil
}

func applyCertificateDelete(next State, record *CertificateRecord) (State, sm.Result, error) {
	if record == nil {
		return next, sm.Result{}, oops.
			In("raftnode").
			New("certificate delete command requires key")
	}
	key := strings.Trim(strings.ReplaceAll(record.Key, "\\", "/"), "/")
	if key == "" {
		next.Certificates = collectionlist.NewList[CertificateRecord]()
		return next, commandResult(next.Version, true, ""), nil
	}
	prefix := key + "/"
	next.Certificates.RemoveIf(func(existing CertificateRecord) bool {
		return existing.Key == key || strings.HasPrefix(existing.Key, prefix)
	})
	return next, commandResult(next.Version, true, ""), nil
}

func applyCertificateLockAcquire(next State, lock *CertificateLockCommand) (State, sm.Result, error) {
	if lock == nil || lock.Name == "" || lock.Owner == "" {
		return next, sm.Result{}, oops.
			In("raftnode").
			New("certificate lock acquire command requires name and owner")
	}
	requestedAt := lock.RequestedAt
	if requestedAt.IsZero() {
		requestedAt = lock.ExpiresAt
	}
	acquired := false
	next.Locks.RemoveIf(func(existing CertificateLockRecord) bool {
		if existing.Name != lock.Name {
			return false
		}
		if existing.Owner == lock.Owner || !existing.ExpiresAt.After(requestedAt) {
			acquired = true
			return true
		}
		return false
	})
	if acquired || !lockExists(next.Locks, lock.Name) {
		next.Locks.Add(CertificateLockRecord{
			Name:      lock.Name,
			Owner:     lock.Owner,
			ExpiresAt: lock.ExpiresAt,
		})
		return next, commandResult(next.Version, true, ""), nil
	}
	return next, commandResult(next.Version, false, "held"), nil
}

func applyCertificateLockRelease(next State, lock *CertificateLockCommand) (State, sm.Result, error) {
	if lock == nil || lock.Name == "" || lock.Owner == "" {
		return next, sm.Result{}, oops.
			In("raftnode").
			New("certificate lock release command requires name and owner")
	}
	released := false
	next.Locks.RemoveIf(func(existing CertificateLockRecord) bool {
		if existing.Name == lock.Name && existing.Owner == lock.Owner {
			released = true
			return true
		}
		return false
	})
	if !released {
		return next, commandResult(next.Version, false, "not_held"), nil
	}
	return next, commandResult(next.Version, true, ""), nil
}

func commandResult(version uint64, ok bool, reason string) sm.Result {
	data, err := json.Marshal(CommandResult{OK: ok, Reason: reason, Version: version})
	if err != nil {
		return sm.Result{Value: version}
	}
	return sm.Result{Value: version, Data: data}
}

func lockExists(locks *collectionlist.List[CertificateLockRecord], name string) bool {
	return locks != nil && locks.AnyMatch(func(_ int, lock CertificateLockRecord) bool {
		return lock.Name == name
	})
}

func cloneCertificates(records *collectionlist.List[CertificateRecord]) *collectionlist.List[CertificateRecord] {
	if records == nil || records.IsEmpty() {
		return collectionlist.NewList[CertificateRecord]()
	}
	copied := collectionlist.NewListWithCapacity[CertificateRecord](records.Len())
	records.Range(func(_ int, record CertificateRecord) bool {
		copied.Add(cloneCertificateRecord(record))
		return true
	})
	return copied
}

func cloneCertificateLocks(records *collectionlist.List[CertificateLockRecord]) *collectionlist.List[CertificateLockRecord] {
	if records == nil || records.IsEmpty() {
		return collectionlist.NewList[CertificateLockRecord]()
	}
	return records.Clone()
}

func cloneCertificateRecord(record CertificateRecord) CertificateRecord {
	record.Value = bytes.Clone(record.Value)
	return record
}
