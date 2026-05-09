package gateway

import (
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/mapper"
)

var adminMapper = mapper.New(
	mapper.Converter(adminAnyMapRaw),
	mapper.Converter(adminStringMapRaw),
)

func adminAnyMapView(values *mapping.Map[string, any]) map[string]any {
	view := map[string]any{}
	if err := adminMapper.MapInto(&view, values); err != nil {
		return adminAnyMapRaw(values)
	}
	return view
}

func adminStringMapView(values *mapping.Map[string, string]) map[string]string {
	view := map[string]string{}
	if err := adminMapper.MapInto(&view, values); err != nil {
		return adminStringMapRaw(values)
	}
	return view
}

func adminPeersView(peers *collectionlist.List[*ClusterPeer]) []map[string]string {
	if peers == nil {
		return []map[string]string{}
	}
	view, err := mapper.Slice[map[string]string](
		peers.Values(),
		mapper.Converter(adminStringMapRaw),
	)
	if err != nil {
		return adminPeersRaw(peers)
	}
	return view
}

func adminAnyMapRaw(values *mapping.Map[string, any]) map[string]any {
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

func adminStringMapRaw(values *mapping.Map[string, string]) map[string]string {
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

func adminPeersRaw(peers *collectionlist.List[*ClusterPeer]) []map[string]string {
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
		return adminAnyMapRaw(typed)
	case *mapping.Map[string, string]:
		return adminStringMapRaw(typed)
	default:
		return value
	}
}
