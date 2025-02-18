package jsonpatch

import (
	"fmt"
)

type MergePolicy int

const (
	// default policy, replace the original with the patch by fields
	// replace when data type are inconsistent or is array, string, int and etc.
	PolicyPatchAndReplace MergePolicy = iota

	// when data type is array, patch them by index
	PolicyArrayIndexPatch

	// just replace
	PolicyDirectReplace
)

type Merger interface {
	Merge(cur, patch *LazyNode, opts *MergeOptions) (*LazyNode, error)
}

type MergersRegistry interface {
	Get(key string) Merger
	Register(key string, merger Merger) bool
}

func NewMerger(mergeFunc func(cur, patch *LazyNode, opts *MergeOptions) (*LazyNode, error)) Merger {
	return &merger{
		merge: mergeFunc,
	}
}

type merger struct {
	merge func(cur, patch *LazyNode, opts *MergeOptions) (*LazyNode, error)
}

func (m *merger) Merge(cur, patch *LazyNode, opts *MergeOptions) (*LazyNode, error) {
	return m.merge(cur, patch, opts)
}

var (
	patchAndReplaceMerger = &merger{
		merge: func(cur, patch *LazyNode, opts *MergeOptions) (*LazyNode, error) {
			curDoc, err := cur.IntoDoc()

			// if cur node is not doc (array, nil, string, int etc.), then just replace it
			if err != nil {
				PruneNulls(patch)
				return patch, nil
			}
			patchDoc, err := patch.IntoDoc()
			// if patch node is not doc, then just replace it
			// else, we merge it
			if err != nil {
				return patch, nil
			}
			for k, v := range *patchDoc {
				if v == nil {
					if opts.MergeMerge {
						(*curDoc)[k] = nil
					} else {
						delete(*curDoc, k)
					}
				} else {
					cur, ok := (*curDoc)[k]
					// if the key doesn't exist in the current document, we just add it
					// else, merge them recursively
					if !ok || cur == nil {
						if !opts.MergeMerge {
							PruneNulls(v)
						}

						(*curDoc)[k] = v
					} else {
						pathCopy := opts.Path
						opts.Path = AppendPath(opts.Path, k)
						(*curDoc)[k], err = opts.Mergers.Get(opts.Path).Merge(cur, v, opts)
						if err != nil {
							return nil, err
						}
						opts.Path = pathCopy
					}
				}
			}
			return cur, nil
		},
	}

	arrayIndexPatchMerger = &merger{
		merge: func(cur, patch *LazyNode, opts *MergeOptions) (*LazyNode, error) {
			curAry, curAryErr := cur.IntoAry()
			patchAry, patchAryErr := patch.IntoAry()

			if curAryErr != nil || patchAryErr != nil {
				return nil, fmt.Errorf("invalid array")
			}

			if curAry == nil {
				PruneAryNulls(patchAry)
				return patch, nil
			}

			if patchAry == nil {
				PruneAryNulls(curAry)
				return cur, nil
			}

			for idx, patchElement := range *patchAry {
				if idx >= len(*curAry) {
					*curAry = append(*curAry, (*patchAry)[idx:len(*patchAry)]...)
					break
				}
				pathCopy := opts.Path
				opts.Path = AppendPath(opts.Path, "-")
				_, err := opts.Mergers.Get(opts.Path).Merge((*curAry)[idx], patchElement, opts)
				if err != nil {
					return nil, err
				}
				opts.Path = pathCopy
			}
			PruneAryNulls(curAry)
			return cur, nil
		},
	}

	replaceMerger = &merger{
		merge: func(cur, patch *LazyNode, opts *MergeOptions) (ret *LazyNode, retErr error) {
			PruneNulls(patch)
			return patch, nil
		},
	}
)

func DefaultMerger(policy MergePolicy) Merger {
	switch policy {
	case PolicyArrayIndexPatch:
		return arrayIndexPatchMerger
	case PolicyDirectReplace:
		return replaceMerger
	case PolicyPatchAndReplace:
		return patchAndReplaceMerger
	default:
		return nil
	}
}

func NewMergersRegistry() MergersRegistry {
	return &mergersRegistry{
		defaultMerger: DefaultMerger(PolicyPatchAndReplace),
		registry:      make(map[string]Merger),
	}
}

type mergersRegistry struct {
	defaultMerger Merger
	registry      map[string]Merger
}

func (r *mergersRegistry) Get(key string) Merger {
	if merger, ok := r.registry[key]; ok {
		return merger
	}
	return r.defaultMerger
}

func (r *mergersRegistry) Register(key string, merger Merger) bool {
	if r.registry == nil {
		r.registry = make(map[string]Merger)
	}
	r.registry[key] = merger
	return true
}

func AppendPath(a, b string) string {
	return fmt.Sprintf("%s/%s", a, b)
}
