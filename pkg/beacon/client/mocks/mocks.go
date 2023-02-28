// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/blocknative/dreamboat/pkg/beacon/client (interfaces: BeaconNode)

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	metrics "github.com/blocknative/dreamboat/metrics"
	client "github.com/blocknative/dreamboat/pkg/beacon/client"
	structs "github.com/blocknative/dreamboat/pkg/structs"
	types "github.com/flashbots/go-boost-utils/types"
	gomock "github.com/golang/mock/gomock"
)

// MockBeaconNode is a mock of BeaconNode interface.
type MockBeaconNode struct {
	ctrl     *gomock.Controller
	recorder *MockBeaconNodeMockRecorder
}

// MockBeaconNodeMockRecorder is the mock recorder for MockBeaconNode.
type MockBeaconNodeMockRecorder struct {
	mock *MockBeaconNode
}

// NewMockBeaconNode creates a new mock instance.
func NewMockBeaconNode(ctrl *gomock.Controller) *MockBeaconNode {
	mock := &MockBeaconNode{ctrl: ctrl}
	mock.recorder = &MockBeaconNodeMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBeaconNode) EXPECT() *MockBeaconNodeMockRecorder {
	return m.recorder
}

// AttachMetrics mocks base method.
func (m *MockBeaconNode) AttachMetrics(arg0 *metrics.Metrics) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AttachMetrics", arg0)
}

// AttachMetrics indicates an expected call of AttachMetrics.
func (mr *MockBeaconNodeMockRecorder) AttachMetrics(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AttachMetrics", reflect.TypeOf((*MockBeaconNode)(nil).AttachMetrics), arg0)
}

// Endpoint mocks base method.
func (m *MockBeaconNode) Endpoint() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Endpoint")
	ret0, _ := ret[0].(string)
	return ret0
}

// Endpoint indicates an expected call of Endpoint.
func (mr *MockBeaconNodeMockRecorder) Endpoint() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Endpoint", reflect.TypeOf((*MockBeaconNode)(nil).Endpoint))
}

// Genesis mocks base method.
func (m *MockBeaconNode) Genesis() (structs.GenesisInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Genesis")
	ret0, _ := ret[0].(structs.GenesisInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Genesis indicates an expected call of Genesis.
func (mr *MockBeaconNodeMockRecorder) Genesis() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Genesis", reflect.TypeOf((*MockBeaconNode)(nil).Genesis))
}

// GetProposerDuties mocks base method.
func (m *MockBeaconNode) GetProposerDuties(arg0 structs.Epoch) (*client.RegisteredProposersResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetProposerDuties", arg0)
	ret0, _ := ret[0].(*client.RegisteredProposersResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetProposerDuties indicates an expected call of GetProposerDuties.
func (mr *MockBeaconNodeMockRecorder) GetProposerDuties(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetProposerDuties", reflect.TypeOf((*MockBeaconNode)(nil).GetProposerDuties), arg0)
}

// KnownValidators mocks base method.
func (m *MockBeaconNode) KnownValidators(arg0 structs.Slot) (client.AllValidatorsResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "KnownValidators", arg0)
	ret0, _ := ret[0].(client.AllValidatorsResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// KnownValidators indicates an expected call of KnownValidators.
func (mr *MockBeaconNodeMockRecorder) KnownValidators(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "KnownValidators", reflect.TypeOf((*MockBeaconNode)(nil).KnownValidators), arg0)
}

// PublishBlock mocks base method.
func (m *MockBeaconNode) PublishBlock(arg0 *types.SignedBeaconBlock) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PublishBlock", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// PublishBlock indicates an expected call of PublishBlock.
func (mr *MockBeaconNodeMockRecorder) PublishBlock(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PublishBlock", reflect.TypeOf((*MockBeaconNode)(nil).PublishBlock), arg0)
}

// SubscribeToHeadEvents mocks base method.
func (m *MockBeaconNode) SubscribeToHeadEvents(arg0 context.Context, arg1 chan client.HeadEvent) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SubscribeToHeadEvents", arg0, arg1)
}

// SubscribeToHeadEvents indicates an expected call of SubscribeToHeadEvents.
func (mr *MockBeaconNodeMockRecorder) SubscribeToHeadEvents(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SubscribeToHeadEvents", reflect.TypeOf((*MockBeaconNode)(nil).SubscribeToHeadEvents), arg0, arg1)
}

// SyncStatus mocks base method.
func (m *MockBeaconNode) SyncStatus() (*client.SyncStatusPayloadData, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SyncStatus")
	ret0, _ := ret[0].(*client.SyncStatusPayloadData)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SyncStatus indicates an expected call of SyncStatus.
func (mr *MockBeaconNodeMockRecorder) SyncStatus() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SyncStatus", reflect.TypeOf((*MockBeaconNode)(nil).SyncStatus))
}