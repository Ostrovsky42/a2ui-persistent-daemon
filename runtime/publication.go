package runtime

func (s *State) NeedsPublish() bool { return s.PublicationGeneration > s.PublishedGeneration }

// Publish records that a renderer has actually published the current runtime
// projection. Renderer redraw generation is deliberately broader than publication
// generation: local interaction may require a frame without requiring a protocol
// publication acknowledgement or changing the authoritative Document revision.
func (s *State) Publish() {
	if s.PublicationGeneration > s.PublishedGeneration {
		s.PublishedGeneration = s.PublicationGeneration
	}
	if s.DirtyRevision > s.PublishedRevision {
		s.PublishedRevision = s.DirtyRevision
	}
}
