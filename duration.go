package otioflat

import "fmt"

var zeroTime = RationalTime{Value: 0, Rate: 1}

// ResolveDuration returns the visible duration of a subset node. SourceRange
// wins because it is the placement trim. Clips then use the media available
// range; gaps use their explicit duration; compositions sum/maximize children.
func ResolveDuration(n *Node) (RationalTime, error) {
	if n == nil {
		return zeroTime, fmt.Errorf("otioflat: cannot resolve duration of nil node")
	}
	if err := validateTimeRange(n.SourceRange); err != nil {
		return zeroTime, err
	}
	if n.SourceRange != nil {
		return n.SourceRange.Duration, nil
	}

	switch {
	case n.IsComposition():
		return compositionDuration(n)
	case n.MediaReference != nil:
		if n.MediaReference.AvailableRange == nil {
			return zeroTime, fmt.Errorf("otioflat: clip %q has neither source_range nor media available_range", nodeLabel(n, -1))
		}
		if err := validateTimeRange(n.MediaReference.AvailableRange); err != nil {
			return zeroTime, err
		}
		return n.MediaReference.AvailableRange.Duration, nil
	case n.Duration != nil:
		if err := validateTime(*n.Duration); err != nil {
			return zeroTime, err
		}
		return *n.Duration, nil
	default:
		return zeroTime, fmt.Errorf("otioflat: node %q has no resolvable duration", nodeLabel(n, -1))
	}
}

func compositionDuration(n *Node) (RationalTime, error) {
	total := zeroTime
	for index, child := range n.Children {
		if child == nil {
			return zeroTime, fmt.Errorf("otioflat: nil child at %s.children[%d]", nodeLabel(n, -1), index)
		}
		duration, err := ResolveDuration(child)
		if err != nil {
			return zeroTime, err
		}
		switch n.Layout() {
		case LayoutParallel:
			total, err = maxTime(total, duration)
		default:
			total, err = addTime(total, duration)
		}
		if err != nil {
			return zeroTime, err
		}
	}
	return total, nil
}
