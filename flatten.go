package otioflat

import "fmt"

// Flatten expands a nested OTIO subset timeline into occurrence-specific leaf
// records. Every item retains its complete parent-sequence path and local
// timing. Pointer cycles are rejected before expansion can recurse forever.
func Flatten(timeline *Timeline) (*FlatDocument, error) {
	if timeline == nil {
		return nil, fmt.Errorf("otioflat: cannot flatten nil timeline")
	}

	flat := &FlatDocument{
		OTIO_SCHEMA:     FlatSchema,
		Version:         1,
		SourceSchema:    timeline.OTIO_SCHEMA,
		Name:            timeline.Name,
		GlobalStartTime: cloneTime(timeline.GlobalStartTime),
		Metadata:        cloneMetadata(timeline.Metadata),
		Items:           make([]*FlatItem, 0),
	}

	root := timeline.Tracks
	if root == nil {
		return flat, nil
	}
	if !root.IsComposition() {
		return nil, fmt.Errorf("otioflat: timeline.tracks must be a composition with a non-nil children slice")
	}

	rootStamp := stampFromNode(root, -1, zeroTime)
	flat.Root = &rootStamp
	state := &flattenState{
		flat:   flat,
		active: make(map[*Node]struct{}),
	}
	if err := state.expand(root, []Stamp{rootStamp}, zeroTime, zeroTime); err != nil {
		return nil, err
	}
	return flat, nil
}

type flattenState struct {
	flat   *FlatDocument
	active map[*Node]struct{}
	stack  []string
}

func (s *flattenState) expand(node *Node, path []Stamp, localStart, globalStart RationalTime) error {
	if node == nil {
		return fmt.Errorf("otioflat: cannot expand nil composition")
	}
	if _, cyclic := s.active[node]; cyclic {
		cyclePath := append(append([]string{}, s.stack...), nodeLabel(node, -1))
		return &CycleError{Path: cyclePath}
	}

	label := nodeLabel(node, -1)
	s.active[node] = struct{}{}
	s.stack = append(s.stack, label)
	defer func() {
		delete(s.active, node)
		s.stack = s.stack[:len(s.stack)-1]
	}()

	if err := validateTimeRange(node.SourceRange); err != nil {
		return err
	}
	if node.MediaReference != nil && node.MediaReference.AvailableRange != nil {
		if err := validateTimeRange(node.MediaReference.AvailableRange); err != nil {
			return err
		}
	}

	cursor := zeroTime
	for index, child := range node.Children {
		if child == nil {
			return fmt.Errorf("otioflat: nil child in %s.children[%d]", label, index)
		}
		if err := validateTimeRange(child.SourceRange); err != nil {
			return fmt.Errorf("otioflat: invalid source_range for child %d of %s: %w", index, label, err)
		}

		childLocalStart := cursor
		if node.Layout() == LayoutParallel {
			childLocalStart = zeroTime
		}
		childGlobalStart, err := addTime(globalStart, childLocalStart)
		if err != nil {
			return err
		}
		childStamp := stampFromNode(child, index, childLocalStart)

		if child.IsComposition() {
			childPath := append(cloneStamps(path), childStamp)
			if len(child.Children) == 0 {
				s.flat.Items = append(s.flat.Items, &FlatItem{
					Stamp:       childStamp,
					GlobalStart: childGlobalStart,
					Path:        cloneStamps(path),
					Marker:      ContainerMarker,
				})
			} else if err := s.expand(child, childPath, childLocalStart, childGlobalStart); err != nil {
				return err
			}
		} else {
			duration, err := ResolveDuration(child)
			if err != nil {
				return err
			}
			s.flat.Items = append(s.flat.Items, &FlatItem{
				Stamp:       childStamp,
				GlobalStart: childGlobalStart,
				Duration:    duration,
				Path:        cloneStamps(path),
			})
		}

		if node.Layout() == LayoutSequential {
			duration, err := ResolveDuration(child)
			if err != nil {
				return err
			}
			cursor, err = addTime(cursor, duration)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func stampFromNode(node *Node, index int, start RationalTime) Stamp {
	return Stamp{
		OTIO_SCHEMA:     node.OTIO_SCHEMA,
		ID:              node.ID,
		Name:            node.Name,
		Layout:          node.NodeLayout,
		Index:           index,
		LocalStart:      start,
		ChildrenPresent: node.Children != nil,
		SourceRange:     cloneRange(node.SourceRange),
		MediaReference:  cloneMedia(node.MediaReference),
		Duration:        cloneTime(node.Duration),
		Metadata:        cloneMetadata(node.Metadata),
	}
}
