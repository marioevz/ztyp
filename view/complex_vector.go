package view

import (
	"fmt"
	. "github.com/protolambda/ztyp/tree"
	"io"
)

type ComplexVectorTypeDef struct {
	ElemType     TypeDef
	VectorLength uint64
	ComplexTypeBase
}

func ComplexVectorType(elemType TypeDef, length uint64) *ComplexVectorTypeDef {
	minSize := uint64(0)
	maxSize := uint64(0)
	size := uint64(0)
	isFixedSize := elemType.IsFixedByteLength()
	if isFixedSize {
		size = length * elemType.TypeByteLength()
		minSize = size
		maxSize = size
	} else {
		minSize = length * (elemType.MinByteLength() + OffsetByteLength)
		maxSize = length * (elemType.MaxByteLength() + OffsetByteLength)
	}
	return &ComplexVectorTypeDef{
		ElemType:     elemType,
		VectorLength: length,
		ComplexTypeBase: ComplexTypeBase{
			MinSize: minSize,
			MaxSize: maxSize,
			Size: size,
			IsFixedSize: isFixedSize,
		},
	}
}

func (td *ComplexVectorTypeDef) FromElements(v... View) (*ComplexVectorView, error) {
	if td.VectorLength != uint64(len(v)) {
		return nil, fmt.Errorf("expected %d elements, got %d", td.VectorLength, len(v))
	}
	nodes := make([]Node, td.VectorLength, td.VectorLength)
	for i, el := range v {
		nodes[i] = el.Backing()
	}
	depth := CoverDepth(td.VectorLength)
	rootNode, _ := SubtreeFillToContents(nodes, depth)
	vecView, _ := td.ViewFromBacking(rootNode, nil)
	return vecView.(*ComplexVectorView), nil
}

func (td *ComplexVectorTypeDef) ElementType() TypeDef {
	return td.ElemType
}

func (td *ComplexVectorTypeDef) Length() uint64 {
	return td.VectorLength
}

func (td *ComplexVectorTypeDef) DefaultNode() Node {
	depth := CoverDepth(td.VectorLength)
	// The same node N times: the node is immutable, so re-use is safe.
	defaultNode := td.ElemType.DefaultNode()
	// can ignore error, depth is derived from length.
	rootNode, _ := SubtreeFillToLength(defaultNode, depth, td.VectorLength)
	return rootNode
}

func (td *ComplexVectorTypeDef) ViewFromBacking(node Node, hook BackingHook) (View, error) {
	depth := CoverDepth(td.VectorLength)
	return &ComplexVectorView{
		SubtreeView: SubtreeView{
			BackedView: BackedView{
				ViewBase: ViewBase{
					TypeDef: td,
				},
				Hook: hook,
				BackingNode: node,
			},
			depth:       depth,
		},
		ComplexVectorTypeDef: td,
	}, nil
}

func (td *ComplexVectorTypeDef) Default(hook BackingHook) View {
	v, _ := td.ViewFromBacking(td.DefaultNode(), hook)
	return v
}

func (td *ComplexVectorTypeDef) New() *ComplexVectorView {
	return td.Default(nil).(*ComplexVectorView)
}

func (td *ComplexVectorTypeDef) Deserialize(r io.Reader, scope uint64) (View, error) {
	if td.IsFixedSize {
		elemSize := td.ElemType.TypeByteLength()
		if td.Size != scope {
			return nil, fmt.Errorf("expected size %d does not match scope %d", td.Size, scope)
		}
		elements := make([]View, td.VectorLength, td.VectorLength)
		for i := uint64(0); i < td.VectorLength; i++ {
			el, err := td.ElemType.Deserialize(r, elemSize)
			if err != nil {
				return nil, err
			}
			elements[i] = el
		}
		return td.FromElements(elements...)
	} else {
		offsets := make([]uint32, td.VectorLength, td.VectorLength)
		prevOffset := uint32(0)
		for i := uint64(0); i < td.VectorLength; i++ {
			offset, err := ReadOffset(r)
			if err != nil {
				return nil, err
			}
			if offset < prevOffset {
				return nil, fmt.Errorf("offset %d for element %d is smaller than previous offset %d", offset, i, prevOffset)
			}
			offsets[i] = offset
			prevOffset = offset
		}
		elements := make([]View, td.VectorLength, td.VectorLength)
		lastIndex := uint32(len(elements) - 1)
		for i := uint32(0); i < lastIndex; i++ {
			size := offsets[i+1] - offsets[i]
			el, err := td.ElemType.Deserialize(r, uint64(size))
			if err != nil {
				return nil, err
			}
			elements[i] = el
		}
		el, err := td.ElemType.Deserialize(r, scope - uint64(offsets[lastIndex]))
		if err != nil {
			return nil, err
		}
		elements[lastIndex] = el
		return td.FromElements(elements...)
	}
}

func (td *ComplexVectorTypeDef) String() string {
	return fmt.Sprintf("Vector[%s, %d]", td.ElemType.String(), td.VectorLength)
}

type ComplexVectorView struct {
	SubtreeView
	*ComplexVectorTypeDef
}

func (tv *ComplexVectorView) Get(i uint64) (View, error) {
	if i >= tv.ComplexVectorTypeDef.VectorLength {
		return nil, fmt.Errorf("cannot get item at element index %d, vector only has %d elements", i, tv.ComplexVectorTypeDef.VectorLength)
	}
	v, err := tv.SubtreeView.GetNode(i)
	if err != nil {
		return nil, err
	}
	return tv.ComplexVectorTypeDef.ElemType.ViewFromBacking(v, tv.ItemHook(i))
}

func (tv *ComplexVectorView) Set(i uint64, v View) error {
	return tv.setNode(i, v.Backing())
}

func (tv *ComplexVectorView) setNode(i uint64, b Node) error {
	if i >= tv.ComplexVectorTypeDef.VectorLength {
		return fmt.Errorf("cannot set item at element index %d, vector only has %d elements", i, tv.ComplexVectorTypeDef.VectorLength)
	}
	return tv.SubtreeView.SetNode(i, b)
}

func (tv *ComplexVectorView) ItemHook(i uint64) BackingHook {
	return func(b Node) error {
		return tv.setNode(i, b)
	}
}

func (tv *ComplexVectorView) Copy() (View, error) {
	tvCopy := *tv
	tvCopy.Hook = nil
	return &tvCopy, nil
}

func (tv *ComplexVectorView) Iter() ElemIter {
	i := uint64(0)
	return ElemIterFn(func() (elem View, ok bool, err error) {
		if i < tv.VectorLength {
			elem, err = tv.Get(i)
			ok = true
			i += 1
			return
		} else {
			return nil, false, nil
		}
	})
}

func (tv *ComplexVectorView) ReadonlyIter() ElemIter {
	return elemReadonlyIter(tv.BackingNode, tv.VectorLength, tv.depth, tv.ElemType)
}

func (tv *ComplexVectorView) ValueByteLength() (uint64, error) {
	if tv.IsFixedSize {
		return tv.Size, nil
	}
	// TODO
	return 0, nil
}

func (tv *ComplexVectorView) Serialize(w io.Writer) error {
	iter := tv.ReadonlyIter()
	if tv.IsFixedSize {
		return serializeComplexFixElemSeries(iter, w)
	} else {
		return serializeComplexVarElemSeries(tv.VectorLength, iter, w)
	}
}
