package jsonpatch

type MergeOptions struct {
	Path       string
	MergeMerge bool
	Mergers    MergersRegistry
}

func NewMergeOptions() *MergeOptions {
	return &MergeOptions{
		Path:       "",
		MergeMerge: false,
		Mergers:    NewMergersRegistry(),
	}
}

func WithMergeMerge(opts *MergeOptions) *MergeOptions {
	opts.MergeMerge = true
	return opts
}

type MergeOptionsFunc func(opts *MergeOptions) *MergeOptions
