package certstore

import (
	"bytes"
	"time"
)

func normalizeObject(object Object, now time.Time) Object {
	object.Key = cleanKey(object.Key)
	object.Value = bytes.Clone(object.Value)
	if object.Modified.IsZero() {
		object.Modified = now
	}
	return object
}

func cloneObject(object Object) Object {
	object.Value = bytes.Clone(object.Value)
	return object
}
