package quotaProvider

import (
	"errors"
	"fmt"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/registry/core/service/allocator"
)

type AllocatorInterface interface {
	Allocate(uint32) error
	AllocateNext() (uint32, error)
	Release(uint32) error

	Has(uint32) bool
}

var (
	ErrFull      = errors.New("range is full")
	ErrAllocated = errors.New("provided projectId is already allocated")
)

type ErrNotInRange struct {
	ValidPrjID string
}

func (e *ErrNotInRange) Error() string {
	return fmt.Sprintf("provided projectID is not in the valid range. The range of valid ports is %s", e.ValidPrjID)
}

var _ AllocatorInterface = &ProjectIDAllocator{}

type ProjectIDAllocator struct {
	PrjIdStart  int
	MaxPrjIdNum int
	alloc       allocator.Interface
}

// Helper that wraps NewPortAllocatorCustom, for creating a range backed by an in-memory store.
func NewAllocator(ProjectIdStart, ProjectIdMaxNum int) *ProjectIDAllocator {
	rangeSpec := fmt.Sprintf("%d-%d", ProjectIdStart, ProjectIdMaxNum-1)
	return &ProjectIDAllocator{
		PrjIdStart:  ProjectIdStart,
		MaxPrjIdNum: ProjectIdMaxNum,
		alloc:       allocator.NewAllocationMap(ProjectIdMaxNum, rangeSpec),
	}
}

func (r *ProjectIDAllocator) Allocate(prjId uint32) error {
	ok, offset := r.contains(int(prjId))
	if !ok {
		// include valid prjId range in error
		validPrjIds := fmt.Sprintf("%d-%d", r.PrjIdStart, r.MaxPrjIdNum-1)
		return &ErrNotInRange{validPrjIds}
	}

	allocated, err := r.alloc.Allocate(offset)
	if err != nil {
		return err
	}
	if !allocated {
		return ErrAllocated
	}
	return nil
}

func (r *ProjectIDAllocator) AllocateNext() (uint32, error) {
	offset, ok, err := r.alloc.AllocateNext()
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrFull
	}
	return uint32(r.PrjIdStart + offset), nil
}

func (r *ProjectIDAllocator) Release(prjId uint32) error {
	ok, offset := r.contains(int(prjId))
	if !ok {
		klog.Warningf("prjId is not in the range when release it. prjId: %v", prjId)
		return nil
	}

	return r.alloc.Release(offset)
}

func (r *ProjectIDAllocator) Has(prjId uint32) bool {
	ok, offset := r.contains(int(prjId))
	if !ok {
		return false
	}

	return r.alloc.Has(offset)
}

func (r *ProjectIDAllocator) contains(prjID int) (bool, int) {
	if prjID >= r.PrjIdStart && prjID < (r.PrjIdStart+r.MaxPrjIdNum) {
		offset := prjID - r.PrjIdStart
		return true, offset
	}

	return false, 0
}
