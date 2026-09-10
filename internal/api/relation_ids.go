package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// RelationIDs is a list of related object IDs.
//
// The Nimbu API returns relation fields as a plain string array unless
// X-Nimbu-Client-Version is set, in which case it uses a Relation wrapper
// with Reference objects. UnmarshalJSON accepts both shapes. MarshalJSON
// always emits a plain []string.
type RelationIDs []string

// UnmarshalJSON accepts []string, arrays of Reference objects, mixed arrays,
// and {"__type":"Relation","objects":[...]} wrappers.
func (r *RelationIDs) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		*r = nil
		return nil
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	switch raw.(type) {
	case []any, map[string]any, nil:
		*r = RelationIDs(ParseRelationIDs(raw))
		return nil
	default:
		return fmt.Errorf("relation: expected array or object, got %T", raw)
	}
}

// MarshalJSON emits a plain JSON string array.
func (r RelationIDs) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	return json.Marshal([]string(r))
}

// ParseRelationIDs extracts IDs from a plain []string, an array of strings
// and/or Reference objects, or a Relation wrapper object.
func ParseRelationIDs(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case []string:
		out := make([]string, 0, len(v))
		for _, id := range v {
			id = strings.TrimSpace(id)
			if id != "" {
				out = append(out, id)
			}
		}
		return out
	case []any:
		return relationIDsFromList(v)
	case map[string]any:
		if objs, ok := v["objects"].([]any); ok {
			return relationIDsFromList(objs)
		}
		if id := relationObjectID(v); id != "" {
			return []string{id}
		}
		return RelationIDs{}
	default:
		return nil
	}
}

func relationIDsFromList(items []any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		switch it := item.(type) {
		case string:
			it = strings.TrimSpace(it)
			if it != "" {
				out = append(out, it)
			}
		case map[string]any:
			if id := relationObjectID(it); id != "" {
				out = append(out, id)
			}
		}
	}
	return out
}

func relationObjectID(obj map[string]any) string {
	if id, ok := obj["id"].(string); ok {
		if id = strings.TrimSpace(id); id != "" {
			return id
		}
	}
	if id, ok := obj["objectId"].(string); ok {
		if id = strings.TrimSpace(id); id != "" {
			return id
		}
	}
	return ""
}

// UnmarshalJSON decodes a role and records whether each relation field was
// expanded. A Relation pointer without an objects key is not an empty list.
func (r *Role) UnmarshalJSON(data []byte) error {
	type roleJSON Role
	var decoded roleJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = Role(decoded)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.CustomersExpanded = relationJSONExpanded(raw["customers"])
	r.ChildrenExpanded = relationJSONExpanded(raw["children"])
	r.ParentsExpanded = relationJSONExpanded(raw["parents"])
	return nil
}

func relationJSONExpanded(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return true
	}
	var obj map[string]any
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return true
	}
	typeName, _ := obj["__type"].(string)
	if !strings.EqualFold(typeName, "Relation") {
		return true
	}
	_, hasObjects := obj["objects"]
	return hasObjects
}
