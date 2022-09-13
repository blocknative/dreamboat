// Code generated by MockGen. DO NOT EDIT.
// Source: relay.go

// Package mock_relay is a generated GoMock package.
package mock_relay

import (
	context "context"
	reflect "reflect"

	pkg "github.com/blocknative/dreamboat/pkg"
	types "github.com/flashbots/go-boost-utils/types"
	gomock "github.com/golang/mock/gomock"
)

// MockState is a mock of State interface.
type MockState struct {
	ctrl     *gomock.Controller
	recorder *MockStateMockRecorder
}

// MockStateMockRecorder is the mock recorder for MockState.
type MockStateMockRecorder struct {
	mock *MockState
}

// NewMockState creates a new mock instance.
func NewMockState(ctrl *gomock.Controller) *MockState {
	mock := &MockState{ctrl: ctrl}
	mock.recorder = &MockStateMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockState) EXPECT() *MockStateMockRecorder {
	return m.recorder
}

// Beacon mocks base method.
func (m *MockState) Beacon() pkg.BeaconState {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Beacon")
	ret0, _ := ret[0].(pkg.BeaconState)
	return ret0
}

// Beacon indicates an expected call of Beacon.
func (mr *MockStateMockRecorder) Beacon() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Beacon", reflect.TypeOf((*MockState)(nil).Beacon))
}

// Datastore mocks base method.
func (m *MockState) Datastore() pkg.Datastore {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Datastore")
	ret0, _ := ret[0].(pkg.Datastore)
	return ret0
}

// Datastore indicates an expected call of Datastore.
func (mr *MockStateMockRecorder) Datastore() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Datastore", reflect.TypeOf((*MockState)(nil).Datastore))
}

// MockBeaconState is a mock of BeaconState interface.
type MockBeaconState struct {
	ctrl     *gomock.Controller
	recorder *MockBeaconStateMockRecorder
}

// MockBeaconStateMockRecorder is the mock recorder for MockBeaconState.
type MockBeaconStateMockRecorder struct {
	mock *MockBeaconState
}

// NewMockBeaconState creates a new mock instance.
func NewMockBeaconState(ctrl *gomock.Controller) *MockBeaconState {
	mock := &MockBeaconState{ctrl: ctrl}
	mock.recorder = &MockBeaconStateMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBeaconState) EXPECT() *MockBeaconStateMockRecorder {
	return m.recorder
}

// HeadSlot mocks base method.
func (m *MockBeaconState) HeadSlot() pkg.Slot {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HeadSlot")
	ret0, _ := ret[0].(pkg.Slot)
	return ret0
}

// HeadSlot indicates an expected call of HeadSlot.
func (mr *MockBeaconStateMockRecorder) HeadSlot() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HeadSlot", reflect.TypeOf((*MockBeaconState)(nil).HeadSlot))
}

// KnownValidators mocks base method.
func (m *MockBeaconState) KnownValidators() map[types.PubkeyHex]struct{} {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "KnownValidators")
	ret0, _ := ret[0].(map[types.PubkeyHex]struct{})
	return ret0
}

// KnownValidators indicates an expected call of KnownValidators.
func (mr *MockBeaconStateMockRecorder) KnownValidators() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "KnownValidators", reflect.TypeOf((*MockBeaconState)(nil).KnownValidators))
}

// KnownValidatorsByIndex mocks base method.
func (m *MockBeaconState) KnownValidatorsByIndex() map[uint64]types.PubkeyHex {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "KnownValidatorsByIndex")
	ret0, _ := ret[0].(map[uint64]types.PubkeyHex)
	return ret0
}

// KnownValidatorsByIndex indicates an expected call of KnownValidatorsByIndex.
func (mr *MockBeaconStateMockRecorder) KnownValidatorsByIndex() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "KnownValidatorsByIndex", reflect.TypeOf((*MockBeaconState)(nil).KnownValidatorsByIndex))
}

// ValidatorsMap mocks base method.
func (m *MockBeaconState) ValidatorsMap() pkg.BuilderGetValidatorsResponseEntrySlice {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ValidatorsMap")
	ret0, _ := ret[0].(pkg.BuilderGetValidatorsResponseEntrySlice)
	return ret0
}

// ValidatorsMap indicates an expected call of ValidatorsMap.
func (mr *MockBeaconStateMockRecorder) ValidatorsMap() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ValidatorsMap", reflect.TypeOf((*MockBeaconState)(nil).ValidatorsMap))
}

// MockRelay is a mock of Relay interface.
type MockRelay struct {
	ctrl     *gomock.Controller
	recorder *MockRelayMockRecorder
}

// MockRelayMockRecorder is the mock recorder for MockRelay.
type MockRelayMockRecorder struct {
	mock *MockRelay
}

// NewMockRelay creates a new mock instance.
func NewMockRelay(ctrl *gomock.Controller) *MockRelay {
	mock := &MockRelay{ctrl: ctrl}
	mock.recorder = &MockRelayMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRelay) EXPECT() *MockRelayMockRecorder {
	return m.recorder
}

// GetHeader mocks base method.
func (m *MockRelay) GetHeader(arg0 context.Context, arg1 pkg.HeaderRequest, arg2 pkg.State) (*types.GetHeaderResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetHeader", arg0, arg1, arg2)
	ret0, _ := ret[0].(*types.GetHeaderResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetHeader indicates an expected call of GetHeader.
func (mr *MockRelayMockRecorder) GetHeader(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetHeader", reflect.TypeOf((*MockRelay)(nil).GetHeader), arg0, arg1, arg2)
}

// GetPayload mocks base method.
func (m *MockRelay) GetPayload(arg0 context.Context, arg1 *types.SignedBlindedBeaconBlock, arg2 pkg.State) (*types.GetPayloadResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPayload", arg0, arg1, arg2)
	ret0, _ := ret[0].(*types.GetPayloadResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetPayload indicates an expected call of GetPayload.
func (mr *MockRelayMockRecorder) GetPayload(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPayload", reflect.TypeOf((*MockRelay)(nil).GetPayload), arg0, arg1, arg2)
}

// GetValidators mocks base method.
func (m *MockRelay) GetValidators(arg0 pkg.State) pkg.BuilderGetValidatorsResponseEntrySlice {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetValidators", arg0)
	ret0, _ := ret[0].(pkg.BuilderGetValidatorsResponseEntrySlice)
	return ret0
}

// GetValidators indicates an expected call of GetValidators.
func (mr *MockRelayMockRecorder) GetValidators(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetValidators", reflect.TypeOf((*MockRelay)(nil).GetValidators), arg0)
}

// RegisterValidator mocks base method.
func (m *MockRelay) RegisterValidator(arg0 context.Context, arg1 []types.SignedValidatorRegistration, arg2 pkg.State) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RegisterValidator", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// RegisterValidator indicates an expected call of RegisterValidator.
func (mr *MockRelayMockRecorder) RegisterValidator(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RegisterValidator", reflect.TypeOf((*MockRelay)(nil).RegisterValidator), arg0, arg1, arg2)
}

// SubmitBlock mocks base method.
func (m *MockRelay) SubmitBlock(arg0 context.Context, arg1 *types.BuilderSubmitBlockRequest, arg2 pkg.State) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SubmitBlock", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// SubmitBlock indicates an expected call of SubmitBlock.
func (mr *MockRelayMockRecorder) SubmitBlock(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SubmitBlock", reflect.TypeOf((*MockRelay)(nil).SubmitBlock), arg0, arg1, arg2)
}
