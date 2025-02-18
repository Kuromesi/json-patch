package jsonpatch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

func PruneNulls(n *LazyNode) {
	sub, err := n.IntoDoc()

	if err == nil {
		PruneDocNulls(sub)
	} else {
		ary, err := n.IntoAry()

		if err == nil {
			PruneAryNulls(ary)
		}
	}
}

func PruneDocNulls(doc *PartialDoc) *PartialDoc {
	for k, v := range *doc {
		if v == nil {
			delete(*doc, k)
		} else {
			PruneNulls(v)
		}
	}

	return doc
}

func PruneAryNulls(ary *PartialArray) *PartialArray {
	newAry := []*LazyNode{}

	for _, v := range *ary {
		if v != nil {
			PruneNulls(v)
		}
		newAry = append(newAry, v)
	}

	*ary = newAry

	return ary
}

var ErrBadJSONDoc = fmt.Errorf("Invalid JSON Document")
var ErrBadJSONPatch = fmt.Errorf("Invalid JSON Patch")
var errBadMergeTypes = fmt.Errorf("Mismatched JSON Documents")

// MergeMergePatches merges two merge patches together, such that
// applying this resulting merged merge patch to a document yields the same
// as merging each merge patch to the document in succession.
// Deprecated: use MergePatch instead, and set MergeOptions.mergeMerge to be true
func MergeMergePatches(patch1Data, patch2Data []byte) ([]byte, error) {
	opts := &MergeOptions{
		MergeMerge: true,
	}
	return doMergePatch(patch1Data, patch2Data, opts)
}

// MergePatch merges the patchData into the docData.
func MergePatch(docData, patchData []byte, fns ...MergeOptionsFunc) ([]byte, error) {
	opts := NewMergeOptions()
	for _, fn := range fns {
		fn(opts)
	}
	return doMergePatch(docData, patchData, opts)
}

func doMergePatch(docData, patchData []byte, opts *MergeOptions) (merged []byte, retErr error) {
	// in case custom mergers cause panics, we make a recover
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic: %v", r)
		}
	}()
	doc := &LazyNode{}

	docErr := json.Unmarshal(docData, doc)

	patch := &LazyNode{}

	patchErr := json.Unmarshal(patchData, patch)

	if _, ok := docErr.(*json.SyntaxError); ok {
		return nil, ErrBadJSONDoc
	}

	if _, ok := patchErr.(*json.SyntaxError); ok {
		return nil, ErrBadJSONPatch
	}

	docIsDoc := doc.TryDoc()
	patchIsDoc := patch.TryDoc()

	docIsAry := doc.TryAry()
	patchIsAry := patch.TryAry()

	// when the raw string is 'null', or is string type
	if docErr == nil && (docIsDoc && doc.doc == nil || docIsAry && doc.ary == nil || !(docIsAry || docIsDoc)) {
		return nil, ErrBadJSONDoc
	}

	if patchErr == nil && (patchIsDoc && patch.doc == nil || patchIsAry && patch.ary == nil || !(patchIsAry || patchIsDoc)) {
		return nil, ErrBadJSONPatch
	}

	// when type is inconsistent, just replace
	if docIsDoc != patchIsDoc || docIsAry != patchIsAry {
		if patchIsDoc {
			if opts.MergeMerge {
				return json.Marshal(&patch.doc)
			}
			return json.Marshal(PruneDocNulls(&patch.doc))
		} else {
			PruneAryNulls(&patch.ary)
			return json.Marshal(&patch.ary)
		}
	}

	_, err := opts.Mergers.Get(opts.Path).Merge(doc, patch, opts)
	if err != nil {
		return nil, err
	}
	return json.Marshal(doc)
}

// resemblesJSONArray indicates whether the byte-slice "appears" to be
// a JSON array or not.
// False-positives are possible, as this function does not check the internal
// structure of the array. It only checks that the outer syntax is present and
// correct.
func resemblesJSONArray(input []byte) bool {
	input = bytes.TrimSpace(input)

	hasPrefix := bytes.HasPrefix(input, []byte("["))
	hasSuffix := bytes.HasSuffix(input, []byte("]"))

	return hasPrefix && hasSuffix
}

// CreateMergePatch will return a merge patch document capable of converting
// the original document(s) to the modified document(s).
// The parameters can be bytes of either two JSON Documents, or two arrays of
// JSON documents.
// The merge patch returned follows the specification defined at http://tools.ietf.org/html/draft-ietf-appsawg-json-merge-patch-07
func CreateMergePatch(originalJSON, modifiedJSON []byte) ([]byte, error) {
	originalResemblesArray := resemblesJSONArray(originalJSON)
	modifiedResemblesArray := resemblesJSONArray(modifiedJSON)

	// Do both byte-slices seem like JSON arrays?
	if originalResemblesArray && modifiedResemblesArray {
		return createArrayMergePatch(originalJSON, modifiedJSON)
	}

	// Are both byte-slices are not arrays? Then they are likely JSON objects...
	if !originalResemblesArray && !modifiedResemblesArray {
		return createObjectMergePatch(originalJSON, modifiedJSON)
	}

	// None of the above? Then return an error because of mismatched types.
	return nil, errBadMergeTypes
}

// createObjectMergePatch will return a merge-patch document capable of
// converting the original document to the modified document.
func createObjectMergePatch(originalJSON, modifiedJSON []byte) ([]byte, error) {
	originalDoc := map[string]interface{}{}
	modifiedDoc := map[string]interface{}{}

	err := json.Unmarshal(originalJSON, &originalDoc)
	if err != nil {
		return nil, ErrBadJSONDoc
	}

	err = json.Unmarshal(modifiedJSON, &modifiedDoc)
	if err != nil {
		return nil, ErrBadJSONDoc
	}

	dest, err := getDiff(originalDoc, modifiedDoc)
	if err != nil {
		return nil, err
	}

	return json.Marshal(dest)
}

// createArrayMergePatch will return an array of merge-patch documents capable
// of converting the original document to the modified document for each
// pair of JSON documents provided in the arrays.
// Arrays of mismatched sizes will result in an error.
func createArrayMergePatch(originalJSON, modifiedJSON []byte) ([]byte, error) {
	originalDocs := []json.RawMessage{}
	modifiedDocs := []json.RawMessage{}

	err := json.Unmarshal(originalJSON, &originalDocs)
	if err != nil {
		return nil, ErrBadJSONDoc
	}

	err = json.Unmarshal(modifiedJSON, &modifiedDocs)
	if err != nil {
		return nil, ErrBadJSONDoc
	}

	total := len(originalDocs)
	if len(modifiedDocs) != total {
		return nil, ErrBadJSONDoc
	}

	result := []json.RawMessage{}
	for i := 0; i < len(originalDocs); i++ {
		original := originalDocs[i]
		modified := modifiedDocs[i]

		patch, err := createObjectMergePatch(original, modified)
		if err != nil {
			return nil, err
		}

		result = append(result, json.RawMessage(patch))
	}

	return json.Marshal(result)
}

// Returns true if the array matches (must be json types).
// As is idiomatic for go, an empty array is not the same as a nil array.
func MatchesArray(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	if (a == nil && b != nil) || (a != nil && b == nil) {
		return false
	}
	for i := range a {
		if !MatchesValue(a[i], b[i]) {
			return false
		}
	}
	return true
}

// Returns true if the values matches (must be json types)
// The types of the values must match, otherwise it will always return false
// If two map[string]interface{} are given, all elements must match.
func MatchesValue(av, bv interface{}) bool {
	if reflect.TypeOf(av) != reflect.TypeOf(bv) {
		return false
	}
	switch at := av.(type) {
	case string:
		bt := bv.(string)
		if bt == at {
			return true
		}
	case float64:
		bt := bv.(float64)
		if bt == at {
			return true
		}
	case bool:
		bt := bv.(bool)
		if bt == at {
			return true
		}
	case nil:
		// Both nil, fine.
		return true
	case map[string]interface{}:
		bt := bv.(map[string]interface{})
		if len(bt) != len(at) {
			return false
		}
		for key := range bt {
			av, aOK := at[key]
			bv, bOK := bt[key]
			if aOK != bOK {
				return false
			}
			if !MatchesValue(av, bv) {
				return false
			}
		}
		return true
	case []interface{}:
		bt := bv.([]interface{})
		return MatchesArray(at, bt)
	}
	return false
}

// getDiff returns the (recursive) difference between a and b as a map[string]interface{}.
func getDiff(a, b map[string]interface{}) (map[string]interface{}, error) {
	into := map[string]interface{}{}
	for key, bv := range b {
		av, ok := a[key]
		// value was added
		if !ok {
			into[key] = bv
			continue
		}
		// If types have changed, replace completely
		if reflect.TypeOf(av) != reflect.TypeOf(bv) {
			into[key] = bv
			continue
		}
		// Types are the same, compare values
		switch at := av.(type) {
		case map[string]interface{}:
			bt := bv.(map[string]interface{})
			dst := make(map[string]interface{}, len(bt))
			dst, err := getDiff(at, bt)
			if err != nil {
				return nil, err
			}
			if len(dst) > 0 {
				into[key] = dst
			}
		case string, float64, bool:
			if !MatchesValue(av, bv) {
				into[key] = bv
			}
		case []interface{}:
			bt := bv.([]interface{})
			if !MatchesArray(at, bt) {
				into[key] = bv
			}
		case nil:
			switch bv.(type) {
			case nil:
				// Both nil, fine.
			default:
				into[key] = bv
			}
		default:
			panic(fmt.Sprintf("Unknown type:%T in key %s", av, key))
		}
	}
	// Now add all deleted values as nil
	for key := range a {
		_, found := b[key]
		if !found {
			into[key] = nil
		}
	}
	return into, nil
}
