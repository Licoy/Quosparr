package codexdesktop

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
)

type object = map[string]json.RawMessage
type partition struct {
	Globals object `json:"globals"`
	Atoms   object `json:"atoms"`
}
type state struct {
	root, renderer, inactive object
	owner                    string
	original                 []byte
}

func (s Sharer) readState() (*state, error) {
	data, _, err := readRegular(s.statePath())
	if err != nil {
		return nil, err
	}
	root, ok := decodeObject(data)
	if !ok || root == nil {
		return nil, ErrUnknownFormat
	}
	if root[rendererKey] == nil && root[contentOwnerKey] != nil {
		return nil, ErrNotPartitioned
	}
	renderer, ok := decodeObject(root[rendererKey])
	if !ok || renderer == nil {
		return nil, ErrUnknownFormat
	}
	inactive, ok := decodeObject(renderer["inactive"])
	if !ok || inactive == nil {
		return nil, ErrUnknownFormat
	}
	var owner *string
	raw, found := renderer["owner"]
	if !found || json.Unmarshal(raw, &owner) != nil {
		return nil, ErrUnknownFormat
	}
	key := ""
	if owner != nil {
		key = *owner
		if key == "" {
			return nil, ErrUnknownFormat
		}
	}
	return &state{root, renderer, inactive, key, data}, nil
}
func (d *state) keys() []string {
	keys := []string{}
	if d.owner != "" {
		keys = append(keys, d.owner)
	}
	for key := range d.inactive {
		if key != d.owner {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}
func (d *state) partition(key string) (partition, bool, error) {
	empty := partition{object{}, object{}}
	if key == d.owner && key != "" {
		atoms, ok := decodeObject(d.root[atomsKey])
		if !ok {
			return empty, false, ErrUnknownFormat
		}
		globals := object{}
		for k, v := range d.root {
			if k != rendererKey && k != atomsKey {
				globals[k] = v
			}
		}
		return partition{globals, orEmpty(atoms)}, true, nil
	}
	raw, found := d.inactive[key]
	if !found {
		return empty, false, nil
	}
	entry, ok := decodeObject(raw)
	if !ok || entry == nil {
		return empty, false, ErrUnknownFormat
	}
	globals, gok := decodeObject(entry["globals"])
	atoms, aok := decodeObject(entry["atoms"])
	if !gok || !aok {
		return empty, false, ErrUnknownFormat
	}
	return partition{orEmpty(globals), orEmpty(atoms)}, true, nil
}
func (d *state) setPartition(key string, p partition) {
	if key == d.owner && key != "" {
		for _, k := range projectGlobals {
			delete(d.root, k)
		}
		for _, k := range projectGlobals {
			if v, ok := p.Globals[k]; ok {
				d.root[k] = v
			}
		}
		d.root[atomsKey] = encode(p.Atoms)
	} else {
		entry, _ := decodeObject(d.inactive[key])
		entry = orEmpty(entry)
		entry["globals"], entry["atoms"] = encode(p.Globals), encode(p.Atoms)
		d.inactive[key] = encode(entry)
	}
	d.renderer["inactive"] = encode(d.inactive)
	d.root[rendererKey] = encode(d.renderer)
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func partitionPrint(p partition) string {
	var value any
	_ = json.Unmarshal(encode(relatedValues(p)), &value)
	return digest(encode(value))
}
func sharedValues(p partition) partition {
	out := partition{object{}, object{}}
	for _, key := range projectGlobals {
		if v, ok := p.Globals[key]; ok {
			out.Globals[key] = v
		}
	}
	for _, key := range projectAtoms {
		if v, ok := p.Atoms[key]; ok {
			out.Atoms[key] = v
		}
	}
	return out
}

// projectless IDs are a validation input, never copied to another account.
func relatedValues(p partition) partition {
	out := sharedValues(p)
	if raw, found := p.Globals["projectless-thread-ids"]; found {
		out.Globals["projectless-thread-ids"] = raw
	}
	return out
}

func decodeObject(raw json.RawMessage) (object, bool) {
	if raw == nil {
		return nil, true
	}
	var o object
	if json.Unmarshal(raw, &o) != nil {
		return nil, false
	}
	return o, true
}
func orEmpty(o object) object {
	if o == nil {
		return object{}
	}
	return o
}
func sameJSON(a, b []byte) bool {
	var va, vb any
	return json.Unmarshal(a, &va) == nil && json.Unmarshal(b, &vb) == nil && reflect.DeepEqual(va, vb)
}
func encode(v any) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}
