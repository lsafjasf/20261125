package ffv1arc

// SnapshotBlock returns a value copy of one block's manifest state.
func (a *Archive) SnapshotBlock(id int) (BlockState, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	b := a.findLocked(id)
	if b == nil {
		return BlockState{}, ErrBlockNotFound
	}
	cp := *b
	cp.Frames = cloneFrames(b.Frames)
	return cp, nil
}

// BlockIDs returns all block ids in manifest order.
func (a *Archive) BlockIDs() []int {
	a.mu.Lock()
	defer a.mu.Unlock()
	ids := make([]int, 0, len(a.man.Blocks))
	for _, b := range a.man.Blocks {
		ids = append(ids, b.ID)
	}
	return ids
}

// FrameCount returns the indexed frame count for a block.
func (a *Archive) FrameCount(id int) (int, error) {
	b, err := a.SnapshotBlock(id)
	if err != nil {
		return 0, err
	}
	return len(b.Frames), nil
}
