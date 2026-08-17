package eventlog

// AppendSpec appends a spec workflow transition fact to the log. It is safe on a
// nil receiver.
func (l *Log) AppendSpec(stage, slug string) {
	if l == nil {
		return
	}
	l.Append(SpecState, SpecFact{Stage: stage, Slug: slug})
}
