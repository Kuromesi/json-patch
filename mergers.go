package jsonpatch

import "fmt"

const (
	POLICY_REPLACE_WITH_PATCH = "replaceWithPatch"
)

var (
	_MergersRegistry = &MergersRegistry{}
	defaultPolicy    = POLICY_REPLACE_WITH_PATCH
)

func init() {
	_MergersRegistry.Register(POLICY_REPLACE_WITH_PATCH, &replaceWithPatchMerger{})
}

type Indexer interface {
	Index(node *LazyNode) (*LazyNode, error)
	Store(node *LazyNode) error
}

type Merger interface {
	Merge(cur, patch *LazyNode, opts *MergeOptions) *LazyNode
}

type MergersRegistry struct {
	registry map[string]Merger
}

func (r *MergersRegistry) Get(key string) Merger {
	if merger, ok := r.registry[key]; ok {
		return merger
	}
	return _MergersRegistry.Get(defaultPolicy)
}

func (r *MergersRegistry) Register(key string, merger Merger) bool {
	if r.registry == nil {
		r.registry = make(map[string]Merger)
	}
	r.registry[key] = merger
	return true
}

func RegisterMerger(key string, merger Merger) bool {
	return _MergersRegistry.Register(key, merger)
}

func GetMerger(key string) Merger {
	return _MergersRegistry.Get(key)
}

func concatenate(a, b string) string {
	return fmt.Sprintf("%s.%s", a, b)
}

type replaceWithPatchMerger struct{}

func (m *replaceWithPatchMerger) Merge(cur, patch *LazyNode, opts *MergeOptions) *LazyNode {
	curDoc, err := cur.IntoDoc()

	// if cur node is not doc (array, nil, string, int etc.), then just replace it
	if err != nil {
		PruneNulls(patch)
		return patch
	}
	patchDoc, err := patch.IntoDoc()
	// if patch node is not doc, then just replace it
	if err != nil {
		return patch
	}
	// else, we merge it
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
				mergeOpts := &MergeOptions{
					MergeMerge: opts.MergeMerge,
					Path:       concatenate(opts.Path, k),
				}
				(*curDoc)[k] = _MergersRegistry.Get(concatenate(opts.Path, k)).Merge(cur, v, mergeOpts)
			}
		}
	}

	return cur
}
