package gateway

import (
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

func adminAnyMapView(values *mapping.Map[string, any]) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	view := make(map[string]any, values.Len())
	values.Range(func(key string, value any) bool {
		view[key] = adminJSONValue(value)
		return true
	})
	return view
}

func adminStringMapView(values *mapping.Map[string, string]) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	view := make(map[string]string, values.Len())
	values.Range(func(key string, value string) bool {
		view[key] = value
		return true
	})
	return view
}

func adminPeersView(peers *collectionlist.List[*ClusterPeer]) []map[string]string {
	if peers == nil {
		return []map[string]string{}
	}
	view := make([]map[string]string, 0, peers.Len())
	peers.Range(func(_ int, peer *ClusterPeer) bool {
		view = append(view, adminStringMapView(peer))
		return true
	})
	return view
}

func adminJSONValue(value any) any {
	switch typed := value.(type) {
	case *mapping.Map[string, any]:
		return adminAnyMapView(typed)
	case *mapping.Map[string, string]:
		return adminStringMapView(typed)
	default:
		return value
	}
}
