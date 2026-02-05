package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSipSessionTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&SipSession{})
	require.NoError(t, err)

	return db
}

func TestSipSession_CRUD(t *testing.T) {
	db := setupSipSessionTestDB(t)

	// 测试创建SIP会话
	session := &SipSession{
		CallID:        "session-12345@example.com",
		Status:        SipSessionStatusPending,
		RemoteRTPAddr: "192.168.1.100:5006",
		LocalRTPAddr:  "192.168.1.101:5004",
		CallIDRef:     "call-12345@example.com",
		CreatedTime:   time.Now(),
		Metadata:      `{"custom": "session data"}`,
	}

	err := CreateSipSession(db, session)
	assert.NoError(t, err)
	assert.NotZero(t, session.ID)
	assert.NotZero(t, session.CreatedAt)

	// 测试读取SIP会话
	retrieved, err := GetSipSessionByCallID(db, "session-12345@example.com")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "session-12345@example.com", retrieved.CallID)
	assert.Equal(t, SipSessionStatusPending, retrieved.Status)
	assert.Equal(t, "192.168.1.100:5006", retrieved.RemoteRTPAddr)
	assert.Equal(t, "192.168.1.101:5004", retrieved.LocalRTPAddr)
	assert.Equal(t, "call-12345@example.com", retrieved.CallIDRef)
	assert.Equal(t, `{"custom": "session data"}`, retrieved.Metadata)

	// 测试更新SIP会话
	activeTime := time.Now()
	retrieved.Status = SipSessionStatusActive
	retrieved.ActiveTime = &activeTime
	err = UpdateSipSession(db, retrieved)
	assert.NoError(t, err)

	updated, err := GetSipSessionByCallID(db, "session-12345@example.com")
	assert.NoError(t, err)
	assert.Equal(t, SipSessionStatusActive, updated.Status)
	assert.NotNil(t, updated.ActiveTime)

	// 测试结束会话
	endTime := time.Now()
	updated.Status = SipSessionStatusEnded
	updated.EndTime = &endTime
	err = UpdateSipSession(db, updated)
	assert.NoError(t, err)

	ended, err := GetSipSessionByCallID(db, "session-12345@example.com")
	assert.NoError(t, err)
	assert.Equal(t, SipSessionStatusEnded, ended.Status)
	assert.NotNil(t, ended.EndTime)

	// 测试删除会话
	err = DeleteSipSessionByCallID(db, "session-12345@example.com")
	assert.NoError(t, err)

	deleted, err := GetSipSessionByCallID(db, "session-12345@example.com")
	assert.Error(t, err)
	assert.Nil(t, deleted)
}

func TestSipSession_StatusTransitions(t *testing.T) {
	db := setupSipSessionTestDB(t)

	// 测试完整的会话状态转换流程
	session := &SipSession{
		CallID:        "status-test@example.com",
		Status:        SipSessionStatusPending,
		RemoteRTPAddr: "192.168.1.100:5006",
		LocalRTPAddr:  "192.168.1.101:5004",
		CreatedTime:   time.Now(),
	}

	err := CreateSipSession(db, session)
	require.NoError(t, err)

	// 状态转换：等待ACK -> 活跃会话
	activeTime := time.Now()
	session.Status = SipSessionStatusActive
	session.ActiveTime = &activeTime
	err = UpdateSipSession(db, session)
	assert.NoError(t, err)

	retrieved, err := GetSipSessionByCallID(db, "status-test@example.com")
	assert.NoError(t, err)
	assert.Equal(t, SipSessionStatusActive, retrieved.Status)
	assert.NotNil(t, retrieved.ActiveTime)

	// 状态转换：活跃会话 -> 已结束
	endTime := time.Now()
	retrieved.Status = SipSessionStatusEnded
	retrieved.EndTime = &endTime
	err = UpdateSipSession(db, retrieved)
	assert.NoError(t, err)

	ended, err := GetSipSessionByCallID(db, "status-test@example.com")
	assert.NoError(t, err)
	assert.Equal(t, SipSessionStatusEnded, ended.Status)
	assert.NotNil(t, ended.EndTime)
}

func TestSipSession_CancelledSession(t *testing.T) {
	db := setupSipSessionTestDB(t)

	// 测试取消的会话
	session := &SipSession{
		CallID:        "cancelled-session@example.com",
		Status:        SipSessionStatusPending,
		RemoteRTPAddr: "192.168.1.100:5006",
		LocalRTPAddr:  "192.168.1.101:5004",
		CreatedTime:   time.Now(),
	}

	err := CreateSipSession(db, session)
	require.NoError(t, err)

	// 取消会话
	endTime := time.Now()
	session.Status = SipSessionStatusCancelled
	session.EndTime = &endTime
	err = UpdateSipSession(db, session)
	assert.NoError(t, err)

	retrieved, err := GetSipSessionByCallID(db, "cancelled-session@example.com")
	assert.NoError(t, err)
	assert.Equal(t, SipSessionStatusCancelled, retrieved.Status)
	assert.NotNil(t, retrieved.EndTime)
}

func TestGetActiveSipSessions(t *testing.T) {
	db := setupSipSessionTestDB(t)

	// 创建不同状态的会话
	sessions := []*SipSession{
		{
			CallID:        "pending-1@example.com",
			Status:        SipSessionStatusPending,
			RemoteRTPAddr: "192.168.1.100:5006",
			LocalRTPAddr:  "192.168.1.101:5004",
			CreatedTime:   time.Now(),
		},
		{
			CallID:        "active-1@example.com",
			Status:        SipSessionStatusActive,
			RemoteRTPAddr: "192.168.1.102:5006",
			LocalRTPAddr:  "192.168.1.101:5008",
			CreatedTime:   time.Now(),
		},
		{
			CallID:        "active-2@example.com",
			Status:        SipSessionStatusActive,
			RemoteRTPAddr: "192.168.1.103:5006",
			LocalRTPAddr:  "192.168.1.101:5010",
			CreatedTime:   time.Now(),
		},
		{
			CallID:        "ended-1@example.com",
			Status:        SipSessionStatusEnded,
			RemoteRTPAddr: "192.168.1.104:5006",
			LocalRTPAddr:  "192.168.1.101:5012",
			CreatedTime:   time.Now(),
		},
	}

	for _, session := range sessions {
		if session.Status == SipSessionStatusActive {
			activeTime := time.Now()
			session.ActiveTime = &activeTime
		}
		if session.Status == SipSessionStatusEnded {
			endTime := time.Now()
			session.EndTime = &endTime
		}
		err := CreateSipSession(db, session)
		require.NoError(t, err)
	}

	// 获取活跃的会话
	activeSessions, err := GetActiveSipSessions(db)
	assert.NoError(t, err)
	assert.Len(t, activeSessions, 2)

	// 验证返回的都是活跃会话
	for _, session := range activeSessions {
		assert.Equal(t, SipSessionStatusActive, session.Status)
		assert.NotNil(t, session.ActiveTime)
	}

	// 验证包含正确的CallID
	callIDs := make([]string, len(activeSessions))
	for i, session := range activeSessions {
		callIDs[i] = session.CallID
	}
	assert.Contains(t, callIDs, "active-1@example.com")
	assert.Contains(t, callIDs, "active-2@example.com")
}

func TestSipSession_UniqueCallID(t *testing.T) {
	db := setupSipSessionTestDB(t)

	// 创建第一个会话
	session1 := &SipSession{
		CallID:        "unique-test@example.com",
		Status:        SipSessionStatusPending,
		RemoteRTPAddr: "192.168.1.100:5006",
		LocalRTPAddr:  "192.168.1.101:5004",
		CreatedTime:   time.Now(),
	}

	err := CreateSipSession(db, session1)
	assert.NoError(t, err)

	// 尝试创建相同CallID的会话（应该失败）
	session2 := &SipSession{
		CallID:        "unique-test@example.com",
		Status:        SipSessionStatusPending,
		RemoteRTPAddr: "192.168.1.102:5006",
		LocalRTPAddr:  "192.168.1.101:5008",
		CreatedTime:   time.Now(),
	}

	err = CreateSipSession(db, session2)
	assert.Error(t, err) // 应该因为唯一约束失败
}

func TestSipSession_RTPAddresses(t *testing.T) {
	db := setupSipSessionTestDB(t)

	session := &SipSession{
		CallID:        "rtp-test@example.com",
		Status:        SipSessionStatusActive,
		RemoteRTPAddr: "203.0.113.10:5006",
		LocalRTPAddr:  "192.168.1.100:5004",
		CreatedTime:   time.Now(),
	}

	activeTime := time.Now()
	session.ActiveTime = &activeTime

	err := CreateSipSession(db, session)
	assert.NoError(t, err)

	retrieved, err := GetSipSessionByCallID(db, "rtp-test@example.com")
	assert.NoError(t, err)
	assert.Equal(t, "203.0.113.10:5006", retrieved.RemoteRTPAddr)
	assert.Equal(t, "192.168.1.100:5004", retrieved.LocalRTPAddr)
}

func TestSipSession_WithCallReference(t *testing.T) {
	db := setupSipSessionTestDB(t)

	session := &SipSession{
		CallID:        "ref-test@example.com",
		Status:        SipSessionStatusPending,
		RemoteRTPAddr: "192.168.1.100:5006",
		LocalRTPAddr:  "192.168.1.101:5004",
		CallIDRef:     "parent-call-12345@example.com",
		CreatedTime:   time.Now(),
	}

	err := CreateSipSession(db, session)
	assert.NoError(t, err)

	retrieved, err := GetSipSessionByCallID(db, "ref-test@example.com")
	assert.NoError(t, err)
	assert.Equal(t, "parent-call-12345@example.com", retrieved.CallIDRef)
}

func TestSipSession_Metadata(t *testing.T) {
	db := setupSipSessionTestDB(t)

	metadata := `{
		"codec": "G.711",
		"bandwidth": "64kbps",
		"encryption": "SRTP",
		"custom_headers": {
			"X-Custom-Header": "value"
		}
	}`

	session := &SipSession{
		CallID:        "metadata-test@example.com",
		Status:        SipSessionStatusActive,
		RemoteRTPAddr: "192.168.1.100:5006",
		LocalRTPAddr:  "192.168.1.101:5004",
		CreatedTime:   time.Now(),
		Metadata:      metadata,
	}

	activeTime := time.Now()
	session.ActiveTime = &activeTime

	err := CreateSipSession(db, session)
	assert.NoError(t, err)

	retrieved, err := GetSipSessionByCallID(db, "metadata-test@example.com")
	assert.NoError(t, err)
	assert.JSONEq(t, metadata, retrieved.Metadata)
}

func TestSipSession_TimeTracking(t *testing.T) {
	db := setupSipSessionTestDB(t)

	createdTime := time.Now()
	session := &SipSession{
		CallID:        "time-test@example.com",
		Status:        SipSessionStatusPending,
		RemoteRTPAddr: "192.168.1.100:5006",
		LocalRTPAddr:  "192.168.1.101:5004",
		CreatedTime:   createdTime,
	}

	err := CreateSipSession(db, session)
	require.NoError(t, err)

	// 激活会话
	activeTime := time.Now()
	session.Status = SipSessionStatusActive
	session.ActiveTime = &activeTime
	err = UpdateSipSession(db, session)
	assert.NoError(t, err)

	// 结束会话
	endTime := time.Now()
	session.Status = SipSessionStatusEnded
	session.EndTime = &endTime
	err = UpdateSipSession(db, session)
	assert.NoError(t, err)

	// 验证时间跟踪
	retrieved, err := GetSipSessionByCallID(db, "time-test@example.com")
	assert.NoError(t, err)
	assert.Equal(t, createdTime.Unix(), retrieved.CreatedTime.Unix())
	assert.NotNil(t, retrieved.ActiveTime)
	assert.NotNil(t, retrieved.EndTime)
	assert.True(t, retrieved.ActiveTime.After(retrieved.CreatedTime))
	assert.True(t, retrieved.EndTime.After(*retrieved.ActiveTime))
}

func TestSipSession_ErrorHandling(t *testing.T) {
	db := setupSipSessionTestDB(t)

	// 测试获取不存在的会话
	notFound, err := GetSipSessionByCallID(db, "nonexistent@example.com")
	assert.Error(t, err)
	assert.Nil(t, notFound)

	// 测试删除不存在的会话
	err = DeleteSipSessionByCallID(db, "nonexistent@example.com")
	assert.NoError(t, err) // DELETE操作即使没有匹配记录也不会报错

	// 测试获取活跃会话（当没有活跃会话时）
	activeSessions, err := GetActiveSipSessions(db)
	assert.NoError(t, err)
	assert.Len(t, activeSessions, 0)
}

func TestSipSession_SoftDelete(t *testing.T) {
	db := setupSipSessionTestDB(t)

	session := &SipSession{
		CallID:        "soft-delete-test@example.com",
		Status:        SipSessionStatusEnded,
		RemoteRTPAddr: "192.168.1.100:5006",
		LocalRTPAddr:  "192.168.1.101:5004",
		CreatedTime:   time.Now(),
	}

	err := CreateSipSession(db, session)
	require.NoError(t, err)

	// 软删除
	err = db.Delete(session).Error
	assert.NoError(t, err)

	// 正常查询应该找不到
	deleted, err := GetSipSessionByCallID(db, "soft-delete-test@example.com")
	assert.Error(t, err)
	assert.Nil(t, deleted)

	// 包含软删除的查询应该能找到
	var unscoped SipSession
	err = db.Unscoped().Where("call_id = ?", "soft-delete-test@example.com").First(&unscoped).Error
	assert.NoError(t, err)
	assert.NotNil(t, unscoped.DeletedAt)
}

func TestSipSession_ConcurrentSessions(t *testing.T) {
	db := setupSipSessionTestDB(t)

	// 创建多个并发会话
	sessionCount := 10
	for i := 0; i < sessionCount; i++ {
		session := &SipSession{
			CallID:        "concurrent-" + string(rune('0'+i)) + "@example.com",
			Status:        SipSessionStatusActive,
			RemoteRTPAddr: "192.168.1." + string(rune('1'+i)) + ":5006",
			LocalRTPAddr:  "192.168.1.101:500" + string(rune('0'+i)),
			CreatedTime:   time.Now(),
		}

		activeTime := time.Now()
		session.ActiveTime = &activeTime

		err := CreateSipSession(db, session)
		require.NoError(t, err)
	}

	// 获取所有活跃会话
	activeSessions, err := GetActiveSipSessions(db)
	assert.NoError(t, err)
	assert.Len(t, activeSessions, sessionCount)

	// 验证每个会话都是活跃状态
	for _, session := range activeSessions {
		assert.Equal(t, SipSessionStatusActive, session.Status)
		assert.NotNil(t, session.ActiveTime)
	}
}

// Benchmark tests
func BenchmarkCreateSipSession(b *testing.B) {
	db := setupSipSessionTestDB(&testing.T{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		session := &SipSession{
			CallID:        "benchmark-" + string(rune(i)) + "@example.com",
			Status:        SipSessionStatusPending,
			RemoteRTPAddr: "192.168.1.100:5006",
			LocalRTPAddr:  "192.168.1.101:5004",
			CreatedTime:   time.Now(),
		}
		CreateSipSession(db, session)
	}
}

func BenchmarkGetSipSessionByCallID(b *testing.B) {
	db := setupSipSessionTestDB(&testing.T{})

	session := &SipSession{
		CallID:        "benchmark@example.com",
		Status:        SipSessionStatusActive,
		RemoteRTPAddr: "192.168.1.100:5006",
		LocalRTPAddr:  "192.168.1.101:5004",
		CreatedTime:   time.Now(),
	}
	activeTime := time.Now()
	session.ActiveTime = &activeTime
	CreateSipSession(db, session)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetSipSessionByCallID(db, "benchmark@example.com")
	}
}

func BenchmarkGetActiveSipSessions(b *testing.B) {
	db := setupSipSessionTestDB(&testing.T{})

	// 创建一些活跃会话
	for i := 0; i < 100; i++ {
		session := &SipSession{
			CallID:        "active-" + string(rune(i)) + "@example.com",
			Status:        SipSessionStatusActive,
			RemoteRTPAddr: "192.168.1.100:5006",
			LocalRTPAddr:  "192.168.1.101:5004",
			CreatedTime:   time.Now(),
		}
		activeTime := time.Now()
		session.ActiveTime = &activeTime
		CreateSipSession(db, session)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetActiveSipSessions(db)
	}
}
