// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/blocknative/dreamboat/api (interfaces: Relay,Registrations)

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	io "io"
	reflect "reflect"

	structs "github.com/blocknative/dreamboat/structs"
	types "github.com/flashbots/go-boost-utils/types"
	gomock "github.com/golang/mock/gomock"
)

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

// GetBlockReceived mocks base method.
func (m *MockRelay) GetBlockReceived(arg0 context.Context, arg1 io.Writer, arg2 structs.SubmissionTraceQuery) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetBlockReceived", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// GetBlockReceived indicates an expected call of GetBlockReceived.
func (mr *MockRelayMockRecorder) GetBlockReceived(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetBlockReceived", reflect.TypeOf((*MockRelay)(nil).GetBlockReceived), arg0, arg1, arg2)
}

// GetHeader mocks base method.
func (m *MockRelay) GetHeader(arg0 context.Context, arg1 *structs.MetricGroup, arg2 structs.UserContent, arg3 structs.HeaderRequest) (structs.GetHeaderResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetHeader", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(structs.GetHeaderResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetHeader indicates an expected call of GetHeader.
func (mr *MockRelayMockRecorder) GetHeader(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetHeader", reflect.TypeOf((*MockRelay)(nil).GetHeader), arg0, arg1, arg2, arg3)
}

// GetPayload mocks base method.
func (m *MockRelay) GetPayload(arg0 context.Context, arg1 *structs.MetricGroup, arg2 structs.UserContent, arg3 structs.SignedBlindedBeaconBlock) (structs.GetPayloadResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPayload", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(structs.GetPayloadResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetPayload indicates an expected call of GetPayload.
func (mr *MockRelayMockRecorder) GetPayload(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPayload", reflect.TypeOf((*MockRelay)(nil).GetPayload), arg0, arg1, arg2, arg3)
}

// GetPayloadDelivered mocks base method.
func (m *MockRelay) GetPayloadDelivered(arg0 context.Context, arg1 io.Writer, arg2 structs.PayloadTraceQuery) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPayloadDelivered", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// GetPayloadDelivered indicates an expected call of GetPayloadDelivered.
func (mr *MockRelayMockRecorder) GetPayloadDelivered(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPayloadDelivered", reflect.TypeOf((*MockRelay)(nil).GetPayloadDelivered), arg0, arg1, arg2)
}

// SubmitBlock mocks base method.
func (m *MockRelay) SubmitBlock(arg0 context.Context, arg1 *structs.MetricGroup, arg2 structs.UserContent, arg3 structs.SubmitBlockRequest) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SubmitBlock", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// SubmitBlock indicates an expected call of SubmitBlock.
func (mr *MockRelayMockRecorder) SubmitBlock(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SubmitBlock", reflect.TypeOf((*MockRelay)(nil).SubmitBlock), arg0, arg1, arg2, arg3)
}

// MockRegistrations is a mock of Registrations interface.
type MockRegistrations struct {
	ctrl     *gomock.Controller
	recorder *MockRegistrationsMockRecorder
}

// MockRegistrationsMockRecorder is the mock recorder for MockRegistrations.
type MockRegistrationsMockRecorder struct {
	mock *MockRegistrations
}

// NewMockRegistrations creates a new mock instance.
func NewMockRegistrations(ctrl *gomock.Controller) *MockRegistrations {
	mock := &MockRegistrations{ctrl: ctrl}
	mock.recorder = &MockRegistrationsMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRegistrations) EXPECT() *MockRegistrationsMockRecorder {
	return m.recorder
}

// GetValidators mocks base method.
func (m *MockRegistrations) GetValidators(arg0 *structs.MetricGroup) structs.BuilderGetValidatorsResponseEntrySlice {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetValidators", arg0)
	ret0, _ := ret[0].(structs.BuilderGetValidatorsResponseEntrySlice)
	return ret0
}

// GetValidators indicates an expected call of GetValidators.
func (mr *MockRegistrationsMockRecorder) GetValidators(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetValidators", reflect.TypeOf((*MockRegistrations)(nil).GetValidators), arg0)
}

// RegisterValidator mocks base method.
func (m *MockRegistrations) RegisterValidator(arg0 context.Context, arg1 *structs.MetricGroup, arg2 []types.SignedValidatorRegistration) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RegisterValidator", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// RegisterValidator indicates an expected call of RegisterValidator.
func (mr *MockRegistrationsMockRecorder) RegisterValidator(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RegisterValidator", reflect.TypeOf((*MockRegistrations)(nil).RegisterValidator), arg0, arg1, arg2)
}

// Registration mocks base method.
func (m *MockRegistrations) Registration(arg0 context.Context, arg1 types.PublicKey) (types.SignedValidatorRegistration, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Registration", arg0, arg1)
	ret0, _ := ret[0].(types.SignedValidatorRegistration)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Registration indicates an expected call of Registration.
func (mr *MockRegistrationsMockRecorder) Registration(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Registration", reflect.TypeOf((*MockRegistrations)(nil).Registration), arg0, arg1)
}
