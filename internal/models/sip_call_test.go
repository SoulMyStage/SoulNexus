package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSipCallTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{}, &Group{}, &SipCall{})
	require.NoError(t, err)

	return db
}

func createTestUserForSipCall(t *testing.T, db *gorm.DB) *User {
	user := &User{
		Email:    "sipcall@example.com",
		Password: "hashedpassword",
	}
	err := db.Create(user).Error
	require.NoError(t, err)
	return user
}

func TestSipCall_CRUD(t *testing.T) {
	db := setupSipCallTestDB(t)
	user := createTestUserForSipCall(t, db)

	// 测试创建SIP通话记录
	sipCall := &SipCall{
		CallID:        "call-12345@example.com",
		Direction:     SipCallDirectionInbound,
		Status:        SipCallStatusCalling,
		FromUsername:  "alice",
		FromURI:       "sip:alice@example.com",
		FromIP:        "192.168.1.100",
		ToUsername:    "bob",
		ToURI:         "sip:bob@example.com",
		ToIP:          "192.168.1.101",
		LocalRTPAddr:  "192.168.1.101:5004",
		RemoteRTPAddr: "192.168.1.100:5006",
		StartTime:     time.Now(),
		UserID:        &user.ID,
		Metadata:      `{"custom": "data"}`,
		Notes:         "Test call record",
	}

	err := CreateSipCall(db, sipCall)
	assert.NoError(t, err)
	assert.NotZero(t, sipCall.ID)
	assert.NotZero(t, sipCall.CreatedAt)

	// 测试读取SIP通话记录
	retrieved, err := GetSipCallByCallID(db, "call-12345@example.com")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "call-12345@example.com", retrieved.CallID)
	assert.Equal(t, SipCallDirectionInbound, retrieved.Direction)
	assert.Equal(t, SipCallStatusCalling, retrieved.Status)
	assert.Equal(t, "alice", retrieved.FromUsername)
	assert.Equal(t, "sip:alice@example.com", retrieved.FromURI)
	assert.Equal(t, "192.168.1.100", retrieved.FromIP)
	assert.Equal(t, "bob", retrieved.ToUsername)
	assert.Equal(t, "sip:bob@example.com", retrieved.ToURI)
	assert.Equal(t, "192.168.1.101", retrieved.ToIP)
	assert.Equal(t, "192.168.1.101:5004", retrieved.LocalRTPAddr)
	assert.Equal(t, "192.168.1.100:5006", retrieved.RemoteRTPAddr)
	assert.Equal(t, user.ID, *retrieved.UserID)

	// 测试更新SIP通话记录
	answerTime := time.Now()
	retrieved.Status = SipCallStatusAnswered
	retrieved.AnswerTime = &answerTime
	retrieved.Duration = 120
	retrieved.RecordURL = "https://example.com/recordings/call-12345.wav"
	retrieved.Transcription = "Hello, this is a test call."
	retrieved.TranscriptionStatus = "completed"

	err = UpdateSipCall(db, retrieved)
	assert.NoError(t, err)

	updated, err := GetSipCallByCallID(db, "call-12345@example.com")
	assert.NoError(t, err)
	assert.Equal(t, SipCallStatusAnswered, updated.Status)
	assert.NotNil(t, updated.AnswerTime)
	assert.Equal(t, 120, updated.Duration)
	assert.Equal(t, "https://example.com/recordings/call-12345.wav", updated.RecordURL)
	assert.Equal(t, "Hello, this is a test call.", updated.Transcription)
	assert.Equal(t, "completed", updated.TranscriptionStatus)

	// 测试结束通话
	time.Sleep(10 * time.Millisecond) // Ensure some time passes
	endTime := time.Now()
	updated.Status = SipCallStatusEnded
	updated.EndTime = &endTime
	updated.Duration = int(endTime.Sub(updated.StartTime).Seconds())
	if updated.Duration == 0 {
		updated.Duration = 1 // Ensure minimum duration for test
	}

	err = UpdateSipCall(db, updated)
	assert.NoError(t, err)

	ended, err := GetSipCallByCallID(db, "call-12345@example.com")
	assert.NoError(t, err)
	assert.Equal(t, SipCallStatusEnded, ended.Status)
	assert.NotNil(t, ended.EndTime)
	assert.Greater(t, ended.Duration, 0)
}

func TestSipCall_StatusTransitions(t *testing.T) {
	db := setupSipCallTestDB(t)
	user := createTestUserForSipCall(t, db)

	// 测试完整的通话状态转换流程
	sipCall := &SipCall{
		CallID:    "status-test@example.com",
		Direction: SipCallDirectionOutbound,
		Status:    SipCallStatusCalling,
		StartTime: time.Now(),
		UserID:    &user.ID,
	}

	err := CreateSipCall(db, sipCall)
	require.NoError(t, err)

	// 状态转换：呼叫中 -> 响铃中
	sipCall.Status = SipCallStatusRinging
	err = UpdateSipCall(db, sipCall)
	assert.NoError(t, err)

	retrieved, err := GetSipCallByCallID(db, "status-test@example.com")
	assert.NoError(t, err)
	assert.Equal(t, SipCallStatusRinging, retrieved.Status)

	// 状态转换：响铃中 -> 已接通
	answerTime := time.Now()
	retrieved.Status = SipCallStatusAnswered
	retrieved.AnswerTime = &answerTime
	err = UpdateSipCall(db, retrieved)
	assert.NoError(t, err)

	answered, err := GetSipCallByCallID(db, "status-test@example.com")
	assert.NoError(t, err)
	assert.Equal(t, SipCallStatusAnswered, answered.Status)
	assert.NotNil(t, answered.AnswerTime)

	// 状态转换：已接通 -> 已结束
	time.Sleep(10 * time.Millisecond) // Ensure some time passes
	endTime := time.Now()
	answered.Status = SipCallStatusEnded
	answered.EndTime = &endTime
	answered.Duration = int(endTime.Sub(answered.StartTime).Seconds())
	if answered.Duration == 0 {
		answered.Duration = 1 // Ensure minimum duration for test
	}
	err = UpdateSipCall(db, answered)
	assert.NoError(t, err)

	ended, err := GetSipCallByCallID(db, "status-test@example.com")
	assert.NoError(t, err)
	assert.Equal(t, SipCallStatusEnded, ended.Status)
	assert.NotNil(t, ended.EndTime)
	assert.Greater(t, ended.Duration, 0)
}

func TestSipCall_FailedCall(t *testing.T) {
	db := setupSipCallTestDB(t)

	// 测试失败的通话
	sipCall := &SipCall{
		CallID:       "failed-call@example.com",
		Direction:    SipCallDirectionOutbound,
		Status:       SipCallStatusFailed,
		StartTime:    time.Now(),
		ErrorCode:    486,
		ErrorMessage: "Busy Here",
	}

	err := CreateSipCall(db, sipCall)
	assert.NoError(t, err)

	retrieved, err := GetSipCallByCallID(db, "failed-call@example.com")
	assert.NoError(t, err)
	assert.Equal(t, SipCallStatusFailed, retrieved.Status)
	assert.Equal(t, 486, retrieved.ErrorCode)
	assert.Equal(t, "Busy Here", retrieved.ErrorMessage)
}

func TestSipCall_CancelledCall(t *testing.T) {
	db := setupSipCallTestDB(t)

	// 测试取消的通话
	sipCall := &SipCall{
		CallID:    "cancelled-call@example.com",
		Direction: SipCallDirectionOutbound,
		Status:    SipCallStatusCalling,
		StartTime: time.Now(),
	}

	err := CreateSipCall(db, sipCall)
	require.NoError(t, err)

	// 取消通话
	sipCall.Status = SipCallStatusCancelled
	endTime := time.Now()
	sipCall.EndTime = &endTime
	err = UpdateSipCall(db, sipCall)
	assert.NoError(t, err)

	retrieved, err := GetSipCallByCallID(db, "cancelled-call@example.com")
	assert.NoError(t, err)
	assert.Equal(t, SipCallStatusCancelled, retrieved.Status)
	assert.NotNil(t, retrieved.EndTime)
}

func TestGetSipCallsByUserID(t *testing.T) {
	db := setupSipCallTestDB(t)
	user := createTestUserForSipCall(t, db)

	// 创建多个通话记录
	calls := []*SipCall{
		{
			CallID:    "call-1@example.com",
			Direction: SipCallDirectionInbound,
			Status:    SipCallStatusEnded,
			StartTime: time.Now().Add(-2 * time.Hour),
			UserID:    &user.ID,
		},
		{
			CallID:    "call-2@example.com",
			Direction: SipCallDirectionOutbound,
			Status:    SipCallStatusEnded,
			StartTime: time.Now().Add(-1 * time.Hour),
			UserID:    &user.ID,
		},
		{
			CallID:    "call-3@example.com",
			Direction: SipCallDirectionInbound,
			Status:    SipCallStatusAnswered,
			StartTime: time.Now(),
			UserID:    &user.ID,
		},
	}

	for _, call := range calls {
		err := CreateSipCall(db, call)
		require.NoError(t, err)
	}

	// 获取用户的通话记录（不限制数量）
	userCalls, err := GetSipCallsByUserID(db, user.ID, 0)
	assert.NoError(t, err)
	assert.Len(t, userCalls, 3)

	// 验证按时间倒序排列
	assert.True(t, userCalls[0].StartTime.After(userCalls[1].StartTime))
	assert.True(t, userCalls[1].StartTime.After(userCalls[2].StartTime))

	// 获取用户的通话记录（限制数量）
	limitedCalls, err := GetSipCallsByUserID(db, user.ID, 2)
	assert.NoError(t, err)
	assert.Len(t, limitedCalls, 2)
	assert.Equal(t, "call-3@example.com", limitedCalls[0].CallID)
	assert.Equal(t, "call-2@example.com", limitedCalls[1].CallID)
}

func TestGetSipCallsByStatus(t *testing.T) {
	db := setupSipCallTestDB(t)
	user := createTestUserForSipCall(t, db)

	// 创建不同状态的通话记录
	statuses := []SipCallStatus{
		SipCallStatusCalling,
		SipCallStatusRinging,
		SipCallStatusAnswered,
		SipCallStatusEnded,
		SipCallStatusFailed,
		SipCallStatusCancelled,
	}

	for i, status := range statuses {
		call := &SipCall{
			CallID:    "call-" + string(rune('1'+i)) + "@example.com",
			Direction: SipCallDirectionInbound,
			Status:    status,
			StartTime: time.Now().Add(-time.Duration(i) * time.Hour),
			UserID:    &user.ID,
		}
		err := CreateSipCall(db, call)
		require.NoError(t, err)
	}

	// 按状态查询
	answeredCalls, err := GetSipCallsByStatus(db, SipCallStatusAnswered, 0)
	assert.NoError(t, err)
	assert.Len(t, answeredCalls, 1)
	assert.Equal(t, SipCallStatusAnswered, answeredCalls[0].Status)

	endedCalls, err := GetSipCallsByStatus(db, SipCallStatusEnded, 0)
	assert.NoError(t, err)
	assert.Len(t, endedCalls, 1)
	assert.Equal(t, SipCallStatusEnded, endedCalls[0].Status)

	failedCalls, err := GetSipCallsByStatus(db, SipCallStatusFailed, 0)
	assert.NoError(t, err)
	assert.Len(t, failedCalls, 1)
	assert.Equal(t, SipCallStatusFailed, failedCalls[0].Status)

	// 测试限制数量
	allCalls, err := GetSipCallsByStatus(db, SipCallStatusCalling, 0)
	assert.NoError(t, err)
	assert.Len(t, allCalls, 1)

	limitedCalls, err := GetSipCallsByStatus(db, SipCallStatusCalling, 1)
	assert.NoError(t, err)
	assert.Len(t, limitedCalls, 1)
}

func TestSipCall_WithGroup(t *testing.T) {
	db := setupSipCallTestDB(t)
	user := createTestUserForSipCall(t, db)

	// 创建组织
	group := &Group{
		Name:      "Test Company",
		Type:      "company",
		CreatorID: user.ID,
	}
	err := db.Create(group).Error
	require.NoError(t, err)

	// 创建组织通话记录
	sipCall := &SipCall{
		CallID:    "group-call@example.com",
		Direction: SipCallDirectionInbound,
		Status:    SipCallStatusEnded,
		StartTime: time.Now(),
		UserID:    &user.ID,
		GroupID:   &group.ID,
	}

	err = CreateSipCall(db, sipCall)
	assert.NoError(t, err)

	// 验证组织关联
	retrieved, err := GetSipCallByCallID(db, "group-call@example.com")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved.GroupID)
	assert.Equal(t, group.ID, *retrieved.GroupID)
}

func TestSipCall_Transcription(t *testing.T) {
	db := setupSipCallTestDB(t)
	user := createTestUserForSipCall(t, db)

	sipCall := &SipCall{
		CallID:              "transcription-call@example.com",
		Direction:           SipCallDirectionInbound,
		Status:              SipCallStatusEnded,
		StartTime:           time.Now(),
		UserID:              &user.ID,
		RecordURL:           "https://example.com/recordings/call.wav",
		TranscriptionStatus: "pending",
	}

	err := CreateSipCall(db, sipCall)
	require.NoError(t, err)

	// 更新转录状态为处理中
	sipCall.TranscriptionStatus = "processing"
	err = UpdateSipCall(db, sipCall)
	assert.NoError(t, err)

	// 完成转录
	sipCall.TranscriptionStatus = "completed"
	sipCall.Transcription = "用户：你好，我想咨询一下产品信息。\n客服：好的，请问您需要了解哪方面的信息？"
	err = UpdateSipCall(db, sipCall)
	assert.NoError(t, err)

	// 验证转录结果
	retrieved, err := GetSipCallByCallID(db, "transcription-call@example.com")
	assert.NoError(t, err)
	assert.Equal(t, "completed", retrieved.TranscriptionStatus)
	assert.Contains(t, retrieved.Transcription, "你好")
	assert.Contains(t, retrieved.Transcription, "产品信息")

	// 测试转录失败
	failedCall := &SipCall{
		CallID:              "failed-transcription@example.com",
		Direction:           SipCallDirectionInbound,
		Status:              SipCallStatusEnded,
		StartTime:           time.Now(),
		UserID:              &user.ID,
		TranscriptionStatus: "failed",
		TranscriptionError:  "Audio quality too low for transcription",
	}

	err = CreateSipCall(db, failedCall)
	assert.NoError(t, err)

	failedRetrieved, err := GetSipCallByCallID(db, "failed-transcription@example.com")
	assert.NoError(t, err)
	assert.Equal(t, "failed", failedRetrieved.TranscriptionStatus)
	assert.Equal(t, "Audio quality too low for transcription", failedRetrieved.TranscriptionError)
}

func TestSipCall_DirectionTypes(t *testing.T) {
	db := setupSipCallTestDB(t)
	user := createTestUserForSipCall(t, db)

	// 测试呼入通话
	inboundCall := &SipCall{
		CallID:    "inbound@example.com",
		Direction: SipCallDirectionInbound,
		Status:    SipCallStatusEnded,
		StartTime: time.Now(),
		UserID:    &user.ID,
	}

	err := CreateSipCall(db, inboundCall)
	assert.NoError(t, err)

	retrieved, err := GetSipCallByCallID(db, "inbound@example.com")
	assert.NoError(t, err)
	assert.Equal(t, SipCallDirectionInbound, retrieved.Direction)

	// 测试呼出通话
	outboundCall := &SipCall{
		CallID:    "outbound@example.com",
		Direction: SipCallDirectionOutbound,
		Status:    SipCallStatusEnded,
		StartTime: time.Now(),
		UserID:    &user.ID,
	}

	err = CreateSipCall(db, outboundCall)
	assert.NoError(t, err)

	retrieved, err = GetSipCallByCallID(db, "outbound@example.com")
	assert.NoError(t, err)
	assert.Equal(t, SipCallDirectionOutbound, retrieved.Direction)
}

func TestSipCall_RTPAddresses(t *testing.T) {
	db := setupSipCallTestDB(t)

	sipCall := &SipCall{
		CallID:        "rtp-test@example.com",
		Direction:     SipCallDirectionInbound,
		Status:        SipCallStatusAnswered,
		StartTime:     time.Now(),
		LocalRTPAddr:  "192.168.1.100:5004",
		RemoteRTPAddr: "203.0.113.10:5006",
	}

	err := CreateSipCall(db, sipCall)
	assert.NoError(t, err)

	retrieved, err := GetSipCallByCallID(db, "rtp-test@example.com")
	assert.NoError(t, err)
	assert.Equal(t, "192.168.1.100:5004", retrieved.LocalRTPAddr)
	assert.Equal(t, "203.0.113.10:5006", retrieved.RemoteRTPAddr)
}

func TestSipCall_ErrorHandling(t *testing.T) {
	db := setupSipCallTestDB(t)

	// 测试获取不存在的通话记录
	notFound, err := GetSipCallByCallID(db, "nonexistent@example.com")
	assert.Error(t, err)
	assert.Nil(t, notFound)

	// 测试获取不存在用户的通话记录
	emptyCalls, err := GetSipCallsByUserID(db, 99999, 0)
	assert.NoError(t, err)
	assert.Len(t, emptyCalls, 0)

	// 测试获取不存在状态的通话记录
	noStatusCalls, err := GetSipCallsByStatus(db, "nonexistent", 0)
	assert.NoError(t, err)
	assert.Len(t, noStatusCalls, 0)
}

// Benchmark tests
func BenchmarkCreateSipCall(b *testing.B) {
	db := setupSipCallTestDB(&testing.T{})
	user := createTestUserForSipCall(&testing.T{}, db)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sipCall := &SipCall{
			CallID:    "benchmark-" + string(rune(i)) + "@example.com",
			Direction: SipCallDirectionInbound,
			Status:    SipCallStatusEnded,
			StartTime: time.Now(),
			UserID:    &user.ID,
		}
		CreateSipCall(db, sipCall)
	}
}

func BenchmarkGetSipCallByCallID(b *testing.B) {
	db := setupSipCallTestDB(&testing.T{})
	user := createTestUserForSipCall(&testing.T{}, db)

	sipCall := &SipCall{
		CallID:    "benchmark@example.com",
		Direction: SipCallDirectionInbound,
		Status:    SipCallStatusEnded,
		StartTime: time.Now(),
		UserID:    &user.ID,
	}
	CreateSipCall(db, sipCall)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetSipCallByCallID(db, "benchmark@example.com")
	}
}
