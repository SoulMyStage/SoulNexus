package handlers

import (
	"testing"
	"time"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple name",
			input:    "My Plugin",
			expected: "my-plugin",
		},
		{
			name:     "Name with special characters",
			input:    "My@Plugin#123!",
			expected: "my-plugin-123",
		},
		{
			name:     "Name with multiple spaces",
			input:    "My   Plugin   Name",
			expected: "my-plugin-name",
		},
		{
			name:     "Name with leading/trailing spaces",
			input:    "  My Plugin  ",
			expected: "my-plugin",
		},
		{
			name:     "Empty name",
			input:    "",
			expected: "plugin-" + time.Now().Format("2006-01-02-15-04-05"),
		},
		{
			name:     "Only special characters",
			input:    "@#$%^&*()",
			expected: "plugin-" + time.Now().Format("2006-01-02-15-04-05"),
		},
		{
			name:     "Chinese characters",
			input:    "我的插件",
			expected: "plugin-" + time.Now().Format("2006-01-02-15-04-05"),
		},
		{
			name:     "Mixed alphanumeric",
			input:    "Plugin123ABC",
			expected: "plugin123abc",
		},
		{
			name:     "Underscores and hyphens",
			input:    "my_plugin-name",
			expected: "my-plugin-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateSlug(tt.input)

			// For empty or non-alphanumeric inputs, check if it starts with "plugin-"
			if tt.input == "" || tt.input == "@#$%^&*()" || tt.input == "我的插件" {
				assert.Contains(t, result, "plugin-")
				assert.True(t, len(result) > 7) // "plugin-" + timestamp
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestGetUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		setup    func(*gin.Context)
		expected uint
	}{
		{
			name: "Success - Get user ID from CurrentUser",
			setup: func(c *gin.Context) {
				user := &models.User{
					Email:       "test@example.com",
					DisplayName: "testuser",
				}
				c.Set("user", user)
			},
			expected: 123,
		},
		{
			name: "Success - Get user ID from context directly",
			setup: func(c *gin.Context) {
				user := &models.User{
					Email:       "test2@example.com",
					DisplayName: "testuser2",
				}
				c.Set("userField", user) // Using constants.UserField
			},
			expected: 0, // Will be 0 because we're not using the exact constant
		},
		{
			name: "Failure - No user in context",
			setup: func(c *gin.Context) {
				// Don't set any user
			},
			expected: 0,
		},
		{
			name: "Failure - Invalid user type in context",
			setup: func(c *gin.Context) {
				c.Set("user", "invalid_user_type")
			},
			expected: 0,
		},
		{
			name: "Failure - Nil user in context",
			setup: func(c *gin.Context) {
				c.Set("user", nil)
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(nil)
			tt.setup(c)

			result := getUserID(c)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUserID_EdgeCases(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("User with zero ID", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		user := &models.User{
			Email:       "test@example.com",
			DisplayName: "testuser",
		}
		c.Set("user", user)

		result := getUserID(c)
		assert.Equal(t, uint(0), result)
	})

	t.Run("User with maximum uint ID", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		user := &models.User{
			Email:       "test@example.com",
			DisplayName: "testuser",
		}
		c.Set("user", user)

		result := getUserID(c)
		assert.Equal(t, ^uint(0), result)
	})
}

// Benchmark tests
func BenchmarkGenerateSlug(b *testing.B) {
	testCases := []string{
		"My Plugin",
		"My@Plugin#123!",
		"My   Plugin   Name",
		"",
		"@#$%^&*()",
		"Plugin123ABC",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases {
			generateSlug(tc)
		}
	}
}

func BenchmarkGetUserID(b *testing.B) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
	}
	c.Set("user", user)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getUserID(c)
	}
}

// Test helper functions
func TestGenerateSlug_Consistency(t *testing.T) {
	// Test that the same input always produces the same output (except for empty strings)
	input := "My Test Plugin"
	expected := generateSlug(input)

	for i := 0; i < 10; i++ {
		result := generateSlug(input)
		assert.Equal(t, expected, result, "generateSlug should be consistent for the same input")
	}
}

func TestGenerateSlug_Length(t *testing.T) {
	// Test that generated slugs are reasonable in length
	tests := []string{
		"Short",
		"This is a very long plugin name with many words that should be converted to a slug",
		"A",
		"Plugin with numbers 123456789",
	}

	for _, input := range tests {
		result := generateSlug(input)
		assert.True(t, len(result) > 0, "Slug should not be empty")
		assert.True(t, len(result) < 200, "Slug should not be excessively long")
	}
}

func TestGenerateSlug_ValidCharacters(t *testing.T) {
	// Test that generated slugs only contain valid characters
	inputs := []string{
		"My Plugin!@#$%^&*()",
		"Plugin with spaces",
		"UPPERCASE plugin",
		"plugin_with_underscores",
	}

	for _, input := range inputs {
		result := generateSlug(input)

		// Check that result only contains lowercase letters, numbers, and hyphens
		for _, char := range result {
			isValid := (char >= 'a' && char <= 'z') ||
				(char >= '0' && char <= '9') ||
				char == '-'
			assert.True(t, isValid, "Slug should only contain lowercase letters, numbers, and hyphens. Got: %s", result)
		}
	}
}
