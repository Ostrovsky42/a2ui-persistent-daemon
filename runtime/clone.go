package runtime

// Clone returns an independent candidate runtime state for transactional agent
// publication. It does not change ownership: Runtime remains the sole owner of
// focus, inputs, table selection and bindings.
func (s *State) Clone() *State {
	return s.clone()
}
