package otioflat

import (
	"fmt"
)

// Unflatten rebuilds the exact nested subset tree encoded by Flatten. Items
// are attached by their recorded sibling index and occurrence path, so repeated
// instances do not overwrite one another.
func Unflatten(flat *FlatDocument) (*Timeline, error) {
	if flat == nil {
		return nil, fmt.Errorf("otioflat: cannot unflatten nil document")
	}
	if flat.OTIO_SCHEMA != FlatSchema {
		return nil, fmt.Errorf("otioflat: unsupported flat schema %q, want %q", flat.OTIO_SCHEMA, FlatSchema)
	}
	if flat.Version != 1 {
		return nil, fmt.Errorf("otioflat: unsupported flat format version %d", flat.Version)
	}

	timeline := &Timeline{
		OTIO_SCHEMA:     flat.SourceSchema,
		Name:            flat.Name,
		GlobalStartTime: cloneTime(flat.GlobalStartTime),
		Metadata:        cloneMetadata(flat.Metadata),
	}
	if flat.Root == nil {
		return timeline, nil
	}

	root, err := nodeFromStamp(flat.Root)
	if err != nil {
		return nil, err
	}
	if flat.Root.ChildrenPresent {
		root.Children = []*Node{}
	}
	timeline.Tracks = root

	for itemNumber, item := range flat.Items {
		if item == nil {
			return nil, fmt.Errorf("otioflat: flat item %d is nil", itemNumber)
		}
		if item.Index < 0 {
			return nil, fmt.Errorf("otioflat: flat item %q has negative sibling index %d", item.ID, item.Index)
		}

		parent, err := ensurePath(root, item.Path)
		if err != nil {
			return nil, fmt.Errorf("otioflat: flat item %q: %w", item.ID, err)
		}

		node, err := nodeFromStamp(&item.Stamp)
		if err != nil {
			return nil, err
		}
		if item.Marker == ContainerMarker && item.ChildrenPresent {
			node.Children = []*Node{}
		}
		if err := attachChild(parent, item.Index, node); err != nil {
			return nil, fmt.Errorf("otioflat: flat item %q: %w", item.ID, err)
		}
	}

	if err := checkNoHoles(root); err != nil {
		return nil, err
	}
	return timeline, nil
}

func ensurePath(root *Node, path []Stamp) (*Node, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("provenance path is empty")
	}

	// path[0] is the root stamp. The caller already created root from
	// FlatDocument.Root, so subsequent stamps are intermediate compositions.
	current := root
	for i := 1; i < len(path); i++ {
		stamp := path[i]
		if stamp.Index < 0 {
			return nil, fmt.Errorf("path frame %q has negative index %d", stamp.ID, stamp.Index)
		}
		next, err := nodeFromStamp(&stamp)
		if err != nil {
			return nil, err
		}
		if stamp.ChildrenPresent {
			next.Children = []*Node{}
		}
		if err := attachChild(current, stamp.Index, next); err != nil {
			return nil, fmt.Errorf("path frame %q: %w", stamp.ID, err)
		}
		if current.Children[stamp.Index] == nil {
			return nil, fmt.Errorf("internal error: child was not attached")
		}
		current = current.Children[stamp.Index]
	}
	return current, nil
}

func attachChild(parent *Node, index int, child *Node) error {
	if parent == nil {
		return fmt.Errorf("parent is nil")
	}
	if parent.Children == nil {
		parent.Children = []*Node{}
	}
	for len(parent.Children) <= index {
		parent.Children = append(parent.Children, nil)
	}
	if existing := parent.Children[index]; existing != nil {
		if nodesConsistent(existing, child) {
			if child.IsComposition() && existing.Children == nil {
				existing.Children = []*Node{}
			}
			return nil
		}
		return fmt.Errorf("conflicting provenance at %s.children[%d]", nodeLabel(parent, -1), index)
	}
	parent.Children[index] = child
	return nil
}

func nodesConsistent(a, b *Node) bool {
	return a.OTIO_SCHEMA == b.OTIO_SCHEMA &&
		a.ID == b.ID &&
		a.Name == b.Name &&
		a.NodeLayout == b.NodeLayout
}

func checkNoHoles(node *Node) error {
	if node == nil || !node.IsComposition() {
		return nil
	}
	for index, child := range node.Children {
		if child == nil {
			return fmt.Errorf("otioflat: reconstructed %s.children[%d] is missing", nodeLabel(node, -1), index)
		}
		if err := checkNoHoles(child); err != nil {
			return err
		}
	}
	return nil
}

func nodeFromStamp(stamp *Stamp) (*Node, error) {
	if stamp == nil {
		return nil, fmt.Errorf("otioflat: cannot reconstruct node from nil stamp")
	}
	return &Node{
		OTIO_SCHEMA:    stamp.OTIO_SCHEMA,
		ID:             stamp.ID,
		Name:           stamp.Name,
		NodeLayout:     stamp.Layout,
		SourceRange:    cloneRange(stamp.SourceRange),
		MediaReference: cloneMedia(stamp.MediaReference),
		Duration:       cloneTime(stamp.Duration),
		Metadata:       cloneMetadata(stamp.Metadata),
	}, nil
}
