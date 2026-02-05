package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupDeviceTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// 迁移所有相关表
	err = db.AutoMigrate(
		&User{},
		&Group{},
		&GroupMember{},
		&Assistant{},
		&Device{},
		&DeviceLifecycle{},
		&DeviceLifecycleHistory{},
		&DeviceMaintenanceRecord{},
		&DeviceLifecycleMetrics{},
		&DeviceErrorLog{},
		&CallRecording{},
		&DevicePerformanceLog{},
	)
	require.NoError(t, err)

	return db
}

func createTestUser(t *testing.T, db *gorm.DB) *User {
	user := &User{
		Email:    "test@example.com",
		Password: "hashedpassword",
	}
	err := db.Create(user).Error
	require.NoError(t, err)
	return user
}

func createTestAssistant(t *testing.T, db *gorm.DB, userID uint) *Assistant {
	assistant := &Assistant{
		UserID:      userID,
		Name:        "Test Assistant",
		Description: "Test Description",
	}
	err := db.Create(assistant).Error
	require.NoError(t, err)
	return assistant
}

func TestFlexibleInt_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected FlexibleInt
		wantErr  bool
	}{
		{
			name:     "parse string number",
			input:    `"123"`,
			expected: FlexibleInt(123),
			wantErr:  false,
		},
		{
			name:     "parse integer",
			input:    `456`,
			expected: FlexibleInt(456),
			wantErr:  false,
		},
		{
			name:     "parse empty string",
			input:    `""`,
			expected: FlexibleInt(0),
			wantErr:  false,
		},
		{
			name:     "parse null",
			input:    `null`,
			expected: FlexibleInt(0),
			wantErr:  false,
		},
		{
			name:    "parse invalid string",
			input:   `"abc"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fi FlexibleInt
			err := json.Unmarshal([]byte(tt.input), &fi)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, fi)
			}
		})
	}
}

func TestFlexibleInt_Int(t *testing.T) {
	fi := FlexibleInt(123)
	result := fi.Int()
	assert.NotNil(t, result)
	assert.Equal(t, 123, *result)
}

func TestCreateDevice(t *testing.T) {
	db := setupDeviceTestDB(t)
	user := createTestUser(t, db)
	assistant := createTestAssistant(t, db, user.ID)

	tests := []struct {
		name    string
		device  *Device
		wantErr bool
	}{
		{
			name: "valid device",
			device: &Device{
				ID:          "test-device-001",
				UserID:      user.ID,
				MacAddress:  "00:11:22:33:44:55",
				DeviceName:  "Test Device",
				Board:       "ESP32",
				AppVersion:  "1.0.0",
				AssistantID: func() *uint { id := uint(assistant.ID); return &id }(),
			},
			wantErr: false,
		},
		{
			name:    "nil device",
			device:  nil,
			wantErr: true,
		},
		{
			name: "empty device ID",
			device: &Device{
				UserID:     user.ID,
				MacAddress: "00:11:22:33:44:56",
			},
			wantErr: true,
		},
		{
			name: "empty MAC address",
			device: &Device{
				ID:     "test-device-002",
				UserID: user.ID,
			},
			wantErr: true,
		},
		{
			name: "zero user ID",
			device: &Device{
				ID:         "test-device-003",
				MacAddress: "00:11:22:33:44:57",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CreateDevice(db, tt.device)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotZero(t, tt.device.CreatedAt)
			}
		})
	}
}

func TestCreateDevice_NilDB(t *testing.T) {
	device := &Device{
		ID:         "test-device",
		UserID:     1,
		MacAddress: "00:11:22:33:44:55",
	}
	err := CreateDevice(nil, device)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database connection is nil")
}

func TestGetDeviceByMacAddress(t *testing.T) {
	db := setupDeviceTestDB(t)
	user := createTestUser(t, db)

	// 创建测试设备
	device := &Device{
		ID:         "test-device-001",
		UserID:     user.ID,
		MacAddress: "00:11:22:33:44:55",
		DeviceName: "Test Device",
	}
	err := CreateDevice(db, device)
	require.NoError(t, err)

	// 测试获取设备
	found, err := GetDeviceByMacAddress(db, "00:11:22:33:44:55")
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, device.ID, found.ID)
	assert.Equal(t, device.MacAddress, found.MacAddress)

	// 测试获取不存在的设备
	notFound, err := GetDeviceByMacAddress(db, "00:11:22:33:44:99")
	assert.Error(t, err)
	assert.Nil(t, notFound)
}

func TestGetDeviceByID(t *testing.T) {
	db := setupDeviceTestDB(t)
	user := createTestUser(t, db)

	device := &Device{
		ID:         "test-device-001",
		UserID:     user.ID,
		MacAddress: "00:11:22:33:44:55",
		DeviceName: "Test Device",
	}
	err := CreateDevice(db, device)
	require.NoError(t, err)

	found, err := GetDeviceByID(db, "test-device-001")
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, device.ID, found.ID)

	notFound, err := GetDeviceByID(db, "nonexistent")
	assert.Error(t, err)
	assert.Nil(t, notFound)
}

func TestUpdateDevice(t *testing.T) {
	db := setupDeviceTestDB(t)
	user := createTestUser(t, db)

	device := &Device{
		ID:         "test-device-001",
		UserID:     user.ID,
		MacAddress: "00:11:22:33:44:55",
		DeviceName: "Test Device",
	}
	err := CreateDevice(db, device)
	require.NoError(t, err)

	// 更新设备
	device.DeviceName = "Updated Device"
	device.Board = "ESP32-S3"
	err = UpdateDevice(db, device)
	assert.NoError(t, err)

	// 验证更新
	updated, err := GetDeviceByID(db, device.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Device", updated.DeviceName)
	assert.Equal(t, "ESP32-S3", updated.Board)
}

func TestDeleteDevice(t *testing.T) {
	db := setupDeviceTestDB(t)
	user := createTestUser(t, db)

	device := &Device{
		ID:         "test-device-001",
		UserID:     user.ID,
		MacAddress: "00:11:22:33:44:55",
		DeviceName: "Test Device",
	}
	err := CreateDevice(db, device)
	require.NoError(t, err)

	// 删除设备
	err = DeleteDevice(db, device.ID)
	assert.NoError(t, err)

	// 验证删除
	deleted, err := GetDeviceByID(db, device.ID)
	assert.Error(t, err)
	assert.Nil(t, deleted)
}

func TestGetUserDevices(t *testing.T) {
	db := setupDeviceTestDB(t)
	user := createTestUser(t, db)
	assistant := createTestAssistant(t, db, user.ID)

	// 创建多个设备
	devices := []*Device{
		{
			ID:          "device-001",
			UserID:      user.ID,
			MacAddress:  "00:11:22:33:44:55",
			DeviceName:  "Device 1",
			AssistantID: func() *uint { id := uint(assistant.ID); return &id }(),
			LastSeen:    &time.Time{},
		},
		{
			ID:         "device-002",
			UserID:     user.ID,
			MacAddress: "00:11:22:33:44:56",
			DeviceName: "Device 2",
			LastSeen:   &time.Time{},
		},
	}

	for _, device := range devices {
		now := time.Now()
		device.LastSeen = &now
		err := CreateDevice(db, device)
		require.NoError(t, err)
	}

	// 测试获取所有设备
	userDevices, err := GetUserDevices(db, user.ID, nil)
	assert.NoError(t, err)
	assert.Len(t, userDevices, 2)

	// 测试按助手ID过滤
	assistantID := uint(assistant.ID)
	assistantDevices, err := GetUserDevices(db, user.ID, &assistantID)
	assert.NoError(t, err)
	assert.Len(t, assistantDevices, 1)
	assert.Equal(t, "device-001", assistantDevices[0].ID)
}

func TestUpdateDeviceStatus(t *testing.T) {
	db := setupDeviceTestDB(t)
	user := createTestUser(t, db)

	device := &Device{
		ID:         "test-device-001",
		UserID:     user.ID,
		MacAddress: "00:11:22:33:44:55",
		DeviceName: "Test Device",
	}
	err := CreateDevice(db, device)
	require.NoError(t, err)

	// 更新状态
	status := map[string]interface{}{
		"cpu_usage":    75.5,
		"memory_usage": 60.2,
		"temperature":  45.0,
	}
	err = UpdateDeviceStatus(db, device.MacAddress, status)
	assert.NoError(t, err)

	// 验证更新
	updated, err := GetDeviceByMacAddress(db, device.MacAddress)
	assert.NoError(t, err)
	assert.Equal(t, 75.5, updated.CPUUsage)
	assert.Equal(t, 60.2, updated.MemoryUsage)
	assert.Equal(t, 45.0, updated.Temperature)
}

func TestUpdateDeviceOnlineStatus(t *testing.T) {
	db := setupDeviceTestDB(t)
	user := createTestUser(t, db)

	device := &Device{
		ID:         "test-device-001",
		UserID:     user.ID,
		MacAddress: "00:11:22:33:44:55",
		DeviceName: "Test Device",
	}
	err := CreateDevice(db, device)
	require.NoError(t, err)

	// 设置在线状态
	err = UpdateDeviceOnlineStatus(db, device.MacAddress, true)
	assert.NoError(t, err)

	// 验证更新
	updated, err := GetDeviceByMacAddress(db, device.MacAddress)
	assert.NoError(t, err)
	assert.True(t, updated.IsOnline)
	assert.NotNil(t, updated.LastSeen)
	assert.NotNil(t, updated.StartTime)

	// 设置离线状态
	err = UpdateDeviceOnlineStatus(db, device.MacAddress, false)
	assert.NoError(t, err)

	updated, err = GetDeviceByMacAddress(db, device.MacAddress)
	assert.NoError(t, err)
	assert.False(t, updated.IsOnline)
}

func TestLogDeviceError(t *testing.T) {
	db := setupDeviceTestDB(t)
	user := createTestUser(t, db)

	device := &Device{
		ID:         "test-device-001",
		UserID:     user.ID,
		MacAddress: "00:11:22:33:44:55",
		DeviceName: "Test Device",
	}
	err := CreateDevice(db, device)
	require.NoError(t, err)

	// 记录错误
	err = LogDeviceError(db, device.ID, device.MacAddress, "NETWORK_ERROR", "ERROR", "NET001", "Connection timeout", "stack trace", "context")
	assert.NoError(t, err)

	// 验证错误日志
	var errorLog DeviceErrorLog
	err = db.Where("device_id = ?", device.ID).First(&errorLog).Error
	assert.NoError(t, err)
	assert.Equal(t, "NETWORK_ERROR", errorLog.ErrorType)
	assert.Equal(t, "NET001", errorLog.ErrorCode)
	assert.Equal(t, "Connection timeout", errorLog.ErrorMsg)

	// 验证设备错误计数更新
	updated, err := GetDeviceByID(db, device.ID)
	assert.NoError(t, err)
	assert.Equal(t, 1, updated.ErrorCount)
	assert.Equal(t, "Connection timeout", updated.LastError)
	assert.NotNil(t, updated.LastErrorAt)
}

func TestLogDevicePerformance(t *testing.T) {
	db := setupDeviceTestDB(t)
	user := createTestUser(t, db)

	device := &Device{
		ID:         "test-device-001",
		UserID:     user.ID,
		MacAddress: "00:11:22:33:44:55",
		DeviceName: "Test Device",
	}
	err := CreateDevice(db, device)
	require.NoError(t, err)

	// 记录性能数据
	err = LogDevicePerformance(db, device.ID, device.MacAddress, 75.5, 60.2, 45.0, 100)
	assert.NoError(t, err)

	// 验证性能日志
	var perfLog DevicePerformanceLog
	err = db.Where("device_id = ?", device.ID).First(&perfLog).Error
	assert.NoError(t, err)
	assert.Equal(t, 75.5, perfLog.CPUUsage)
	assert.Equal(t, 60.2, perfLog.MemoryUsage)
	assert.Equal(t, 45.0, perfLog.Temperature)
	assert.Equal(t, 100, perfLog.NetworkLatency)

	// 验证设备性能状态更新
	updated, err := GetDeviceByID(db, device.ID)
	assert.NoError(t, err)
	assert.Equal(t, 75.5, updated.CPUUsage)
	assert.Equal(t, 60.2, updated.MemoryUsage)
	assert.Equal(t, 45.0, updated.Temperature)
}

func TestGetDevicePerformanceHistory(t *testing.T) {
	db := setupDeviceTestDB(t)
	user := createTestUser(t, db)

	device := &Device{
		ID:         "test-device-001",
		UserID:     user.ID,
		MacAddress: "00:11:22:33:44:55",
		DeviceName: "Test Device",
	}
	err := CreateDevice(db, device)
	require.NoError(t, err)

	// 记录多个性能数据点
	for i := 0; i < 5; i++ {
		err = LogDevicePerformance(db, device.ID, device.MacAddress, float64(70+i), float64(50+i), float64(40+i), 100+i)
		assert.NoError(t, err)
		time.Sleep(time.Millisecond) // 确保时间戳不同
	}

	// 获取性能历史
	history, err := GetDevicePerformanceHistory(db, device.ID, 24)
	assert.NoError(t, err)
	assert.Len(t, history, 5)

	// 验证数据按时间排序
	for i := 1; i < len(history); i++ {
		assert.True(t, history[i].RecordedAt.After(history[i-1].RecordedAt) || history[i].RecordedAt.Equal(history[i-1].RecordedAt))
	}
}

func TestCallRecording_ConversationDetails(t *testing.T) {
	recording := &CallRecording{}

	// 测试设置和获取对话详情
	details := &ConversationDetails{
		SessionID:  "session-001",
		StartTime:  time.Now(),
		EndTime:    time.Now().Add(time.Minute),
		TotalTurns: 5,
		UserTurns:  3,
		AITurns:    2,
		Turns: []ConversationTurn{
			{
				TurnID:    1,
				Timestamp: time.Now(),
				Type:      "user",
				Content:   "Hello",
				Duration:  1000,
			},
		},
	}

	err := recording.SetConversationDetails(details)
	assert.NoError(t, err)
	assert.NotEmpty(t, recording.ConversationDetailsJSON)

	retrieved, err := recording.GetConversationDetails()
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, details.SessionID, retrieved.SessionID)
	assert.Equal(t, details.TotalTurns, retrieved.TotalTurns)

	// 测试设置为nil
	err = recording.SetConversationDetails(nil)
	assert.NoError(t, err)
	assert.Empty(t, recording.ConversationDetailsJSON)

	retrieved, err = recording.GetConversationDetails()
	assert.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestCallRecording_TimingMetrics(t *testing.T) {
	recording := &CallRecording{}

	// 测试设置和获取时间指标
	metrics := &TimingMetrics{
		SessionDuration:      60000,
		ASRCalls:             3,
		ASRTotalTime:         1500,
		ASRAverageTime:       500,
		LLMCalls:             2,
		LLMTotalTime:         3000,
		LLMAverageTime:       1500,
		ResponseDelays:       []int64{200, 300, 250},
		AverageResponseDelay: 250,
	}

	err := recording.SetTimingMetrics(metrics)
	assert.NoError(t, err)
	assert.NotEmpty(t, recording.TimingMetricsJSON)

	retrieved, err := recording.GetTimingMetrics()
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, metrics.SessionDuration, retrieved.SessionDuration)
	assert.Equal(t, metrics.ASRCalls, retrieved.ASRCalls)
	assert.Equal(t, metrics.LLMCalls, retrieved.LLMCalls)

	// 测试设置为nil
	err = recording.SetTimingMetrics(nil)
	assert.NoError(t, err)
	assert.Empty(t, recording.TimingMetricsJSON)

	retrieved, err = recording.GetTimingMetrics()
	assert.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestCreateCallRecording(t *testing.T) {
	db := setupDeviceTestDB(t)
	user := createTestUser(t, db)
	assistant := createTestAssistant(t, db, user.ID)

	recording := &CallRecording{
		UserID:      user.ID,
		AssistantID: uint(assistant.ID),
		DeviceID:    "test-device-001",
		MacAddress:  "00:11:22:33:44:55",
		SessionID:   "session-001",
		AudioPath:   "/path/to/audio.wav",
		AudioFormat: "wav",
		Duration:    60,
		CallType:    "voice",
		CallStatus:  "completed",
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(time.Minute),
	}

	err := CreateCallRecording(db, recording)
	assert.NoError(t, err)
	assert.NotZero(t, recording.ID)
}

func TestGetCallRecordingsByAssistant(t *testing.T) {
	db := setupDeviceTestDB(t)
	user := createTestUser(t, db)
	assistant := createTestAssistant(t, db, user.ID)

	// 创建多个录音记录
	for i := 0; i < 3; i++ {
		recording := &CallRecording{
			UserID:      user.ID,
			AssistantID: uint(assistant.ID),
			DeviceID:    "test-device-001",
			SessionID:   "session-" + string(rune('0'+i)),
			AudioPath:   "/path/to/audio.wav",
			Duration:    60,
			StartTime:   time.Now(),
			EndTime:     time.Now().Add(time.Minute),
		}
		err := CreateCallRecording(db, recording)
		require.NoError(t, err)
	}

	recordings, total, err := GetCallRecordingsByAssistant(db, user.ID, uint(assistant.ID), 10, 0)
	assert.NoError(t, err)
	assert.Len(t, recordings, 3)
	assert.Equal(t, int64(3), total)
}

func TestGetCallRecordingsByDevice(t *testing.T) {
	db := setupDeviceTestDB(t)
	user := createTestUser(t, db)
	assistant := createTestAssistant(t, db, user.ID)

	macAddress := "00:11:22:33:44:55"

	// 创建多个录音记录
	for i := 0; i < 2; i++ {
		recording := &CallRecording{
			UserID:      user.ID,
			AssistantID: uint(assistant.ID),
			MacAddress:  macAddress,
			SessionID:   "session-" + string(rune('0'+i)),
			AudioPath:   "/path/to/audio.wav",
			Duration:    60,
			StartTime:   time.Now(),
			EndTime:     time.Now().Add(time.Minute),
		}
		err := CreateCallRecording(db, recording)
		require.NoError(t, err)
	}

	recordings, total, err := GetCallRecordingsByDevice(db, user.ID, macAddress, 10, 0)
	assert.NoError(t, err)
	assert.Len(t, recordings, 2)
	assert.Equal(t, int64(2), total)
}

// Benchmark tests
func BenchmarkCreateDevice(b *testing.B) {
	db := setupDeviceTestDB(&testing.T{})
	user := createTestUser(&testing.T{}, db)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		device := &Device{
			ID:         "device-" + string(rune(i)),
			UserID:     user.ID,
			MacAddress: "00:11:22:33:44:" + string(rune(55+i%10)),
			DeviceName: "Benchmark Device",
		}
		CreateDevice(db, device)
	}
}

func BenchmarkGetDeviceByMacAddress(b *testing.B) {
	db := setupDeviceTestDB(&testing.T{})
	user := createTestUser(&testing.T{}, db)

	device := &Device{
		ID:         "benchmark-device",
		UserID:     user.ID,
		MacAddress: "00:11:22:33:44:55",
		DeviceName: "Benchmark Device",
	}
	CreateDevice(db, device)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetDeviceByMacAddress(db, "00:11:22:33:44:55")
	}
}
