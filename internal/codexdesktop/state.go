package codexdesktop

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
)

// object is a JSON object whose values are kept verbatim.
type object = map[string]json.RawMessage

// shareState returns data with the owner's project keys written into the
// target's partition, and the fingerprint of the target's project values.
// Data comes back unchanged, with no fingerprint when nothing was shared,
// if the app does not isolate accounts, nobody is signed in or the target is
// already the owner. written holds the fingerprints of earlier shares.
func shareState(data []byte, target string, written map[string]string) ([]byte, string, error) {
	doc, ok := decodeObject(data)
	if !ok || doc == nil {
		return nil, "", ErrUnknownFormat
	}
	raw, found := doc[rendererKey]
	if !found {
		return data, "", nil
	}
	renderer, ok := decodeObject(raw)
	if !ok || renderer == nil {
		return nil, "", ErrUnknownFormat
	}
	var owner *string
	ownerRaw, hasOwner := renderer["owner"]
	inactive, ok := decodeObject(renderer["inactive"])
	if !hasOwner || json.Unmarshal(ownerRaw, &owner) != nil || !ok || inactive == nil {
		return nil, "", ErrUnknownFormat
	}
	if owner == nil || *owner == target {
		return data, "", nil
	}
	entry := object{}
	if old, found := inactive[target]; found {
		if entry, ok = decodeObject(old); !ok || entry == nil {
			return nil, "", ErrUnknownFormat
		}
	}
	atoms, okAtoms := decodeObject(entry["atoms"])
	globals, okGlobals := decodeObject(entry["globals"])
	ownerAtoms, okOwner := decodeObject(doc[atomsKey])
	if !okAtoms || !okGlobals || !okOwner {
		return nil, "", ErrUnknownFormat
	}
	atoms, globals = orEmpty(atoms), orEmpty(globals)
	replace := written[target] != "" && written[target] == fingerprint(globals, atoms)
	for _, key := range projectGlobals {
		carry(globals, key, doc, replace)
	}
	for _, key := range projectAtoms {
		carry(atoms, key, ownerAtoms, replace)
	}
	if _, found := inactive[target]; !found && len(atoms) == 0 && len(globals) == 0 {
		return data, "", nil
	}
	print := fingerprint(globals, atoms)
	entry["atoms"], entry["globals"] = encode(atoms), encode(globals)
	next := encode(entry)
	if sameJSON(next, inactive[target]) {
		return data, print, nil
	}
	inactive[target] = next
	renderer["inactive"] = encode(inactive)
	doc[rendererKey] = encode(renderer)
	return encode(doc), print, nil
}

// fingerprint hashes the shared project values of a partition, independent
// of key order and spacing.
func fingerprint(globals, atoms object) string {
	values := map[string]any{}
	add := func(prefix string, from object, keys []string) {
		for _, key := range keys {
			var v any
			if raw, ok := from[key]; ok && json.Unmarshal(raw, &v) == nil {
				values[prefix+key] = v
			}
		}
	}
	add("globals/", globals, projectGlobals)
	add("atoms/", atoms, projectAtoms)
	data, _ := json.Marshal(values) // maps marshal with sorted keys
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// carry sets dst[key] from src: as is when replacing (a missing source value
// removes it), otherwise merged with what dst already holds.
func carry(dst object, key string, src object, replace bool) {
	value, found := src[key]
	switch {
	case !found && replace:
		delete(dst, key)
	case !found:
	case replace:
		dst[key] = value
	default:
		dst[key] = union(value, dst[key])
	}
}

// union merges two objects (src wins on a shared key) or two arrays (src
// first, then dst's other items); any other pair yields src.
func union(src, dst json.RawMessage) json.RawMessage {
	if dst == nil {
		return src
	}
	so, okS := decodeObject(src)
	do, okD := decodeObject(dst)
	if okS && okD && so != nil && do != nil {
		for k, v := range so {
			do[k] = v
		}
		return encode(do)
	}
	var sa, da []json.RawMessage
	if json.Unmarshal(src, &sa) == nil && json.Unmarshal(dst, &da) == nil && sa != nil && da != nil {
		seen := map[string]bool{}
		for _, v := range sa {
			seen[string(compact(v))] = true
		}
		for _, v := range da {
			if !seen[string(compact(v))] {
				sa = append(sa, v)
			}
		}
		return encode(sa)
	}
	return src
}

// decodeObject reads a JSON object; a missing or null value is a nil map,
// and anything else that is not an object fails.
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

// sameJSON compares two documents regardless of key order and spacing.
func sameJSON(a, b []byte) bool {
	var va, vb any
	return json.Unmarshal(a, &va) == nil && json.Unmarshal(b, &vb) == nil && reflect.DeepEqual(va, vb)
}

func compact(raw json.RawMessage) []byte {
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return raw
	}
	return buf.Bytes()
}

// encode writes JSON without HTML escaping, as JavaScript's JSON.stringify.
func encode(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err) // only maps, slices and raw JSON already validated are encoded
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}
