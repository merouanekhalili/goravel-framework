// path/to/goravel/framework/session/driver/file_test.go
package driver

import (
	"os" // Needed for os.IsNotExist
	"path/filepath"
	"testing"

	// Needed for carbon durations
	sessioncontract "github.com/goravel/framework/contracts/session" // Import session contract
	"github.com/goravel/framework/support/carbon"
	"github.com/goravel/framework/support/file"
	"github.com/stretchr/testify/suite"
)

type FileTestSuite struct {
	suite.Suite
}

func TestFileTestSuite(t *testing.T) {
	suite.Run(t, &FileTestSuite{})
}

// BeforeTest cleans up the test directory before each test runs.
func (f *FileTestSuite) BeforeTest(suiteName, testName string) {
	// Use f.getPath() defined in the suite
	err := file.Remove(f.getPath())
	// It's okay if the directory didn't exist before the test
	f.Require().True(err == nil || os.IsNotExist(err), "Failed to clean test directory '%s' before test: %v", f.getPath(), err)
}

// AfterTest cleans up the test directory after each test runs.
func (f *FileTestSuite) AfterTest(suiteName, testName string) {
	err := file.Remove(f.getPath())
	f.Require().True(err == nil || os.IsNotExist(err), "Failed to clean test directory '%s' after test: %v", f.getPath(), err)
}

// TestNewFile_EmptyPathError verifies the internal constructor rejects empty paths.
func (f *FileTestSuite) TestNewFile_EmptyPathError() {
	driver, err := newFile("", f.getMinutes()) // Pass empty path
	f.Error(err, "newFile should return error for empty path")
	f.Nil(driver, "Driver should be nil on error")
	f.Contains(err.Error(), "session file path cannot be empty", "Error message mismatch")
}

// TestClose tests the Close method (currently a no-op).
func (f *FileTestSuite) TestClose() {
	driver := f.getDriver()
	f.Require().NotNil(driver)
	f.Nil(driver.Close())
}

// TestDestroy tests removing session data.
func (f *FileTestSuite) TestDestroy() {
	driver := f.getDriver()

	f.Nil(driver.Destroy("foo"))

	f.Nil(driver.Write("foo", "bar"))
	value, err := driver.Read("foo")
	f.Nil(err)
	f.Equal("bar", value)

	f.Nil(driver.Destroy("foo"))

	value, err = driver.Read("foo")
	f.Nil(err)
	f.Equal("", value)
}

// TestGc tests the garbage collection logic based on time.
func (f *FileTestSuite) TestGc() {
	driver := f.getDriver()
	f.Require().NotNil(driver)
	lifetimeSeconds := f.getMinutes() * 60

	// Write initial data
	sessionIDValid := "gc_valid_session"
	f.Nil(driver.Write(sessionIDValid, "this session should survive gc"))

	f.Nil(driver.Gc(lifetimeSeconds))

	// Verify sessionIDValid still exists
	valueValid, errValid := driver.Read(sessionIDValid)
	f.Nil(errValid)
	f.Equal("this session should survive gc", valueValid, "Valid session removed by GC")

	// Write initial data
	sessionIDExpired := "gc_expired_session"
	f.Nil(driver.Write(sessionIDExpired, "this session should be removed by gc"))
	f.True(file.Exists(filepath.Join(f.getPath(), sessionIDValid)), "Expired session file missing after GC")

	// Simulate time passing so only sessionIDExpired is old enough
	carbon.SetTestNow(carbon.Now(carbon.UTC).AddMinutes(f.getMinutes()).AddMinutes(20))
	defer carbon.UnsetTestNow()

	// Run GC - Should remove sessionIDExpired but not sessionIDValid
	f.Nil(driver.Gc(lifetimeSeconds))

	// Verify sessionIDExpired is gone
	valueExpired, errExpired := driver.Read(sessionIDExpired)
	f.Nil(errExpired, "Read on GC'd session should not error")
	f.Equal("", valueExpired, "Expired session not removed by GC")
	f.False(file.Exists(filepath.Join(f.getPath(), sessionIDExpired)), "Valid session file still exists after GC")
}

// TestGc_NonExistentPath tests GC when the base storage path doesn't exist.
func (f *FileTestSuite) TestGc_NonExistentPath() {
	driver := f.getDriver() // Creates driver and ensures path/test initially exists
	f.Require().NotNil(driver)
	lifetimeSeconds := f.getMinutes() * 60

	// Manually remove the base path AFTER driver creation
	err := file.Remove(f.getPath())
	f.Require().True(err == nil || os.IsNotExist(err), "Failed to remove test directory during test setup")
	f.False(file.Exists(f.getPath()), "Test directory should be gone before calling Gc")

	// Call Gc - it should handle the missing path gracefully without error
	gcErr := driver.Gc(lifetimeSeconds)
	f.Nil(gcErr, "Gc should not return error for non-existent base path")
}

// TestOpen tests the Open method (currently a no-op).
func (f *FileTestSuite) TestOpen() {
	driver := f.getDriver()
	f.Require().NotNil(driver)
	f.Nil(driver.Open("", "")) // Parameters are usually ignored by file driver
}

// TestRead tests reading session data, including expiration checks.
func (f *FileTestSuite) TestRead() {
	driver := f.getDriver()
	f.Require().NotNil(driver)
	sessionID := "read_test_session"
	sessionData := "data to be read"

	// 1. Read non-existent session
	value, err := driver.Read("read_non_existent")
	f.Nil(err)
	f.Equal("", value, "Reading non-existent session should return empty string")

	// 2. Write and read back immediately
	f.Nil(driver.Write(sessionID, sessionData))
	value, err = driver.Read(sessionID)
	f.Nil(err)
	f.Equal(sessionData, value, "Failed to read back recently written data")

	// 3. Test boundary condition - just before expiry
	carbon.SetTestNow(carbon.Now(carbon.UTC).AddMinutes(f.getMinutes()).AddSeconds(-1)) // 1 second before expiry
	value, err = driver.Read(sessionID)
	f.Nil(err)
	f.Equal(sessionData, value, "Session expired too early")
	carbon.UnsetTestNow() // Clean up time travel

	// 4. Test boundary condition - just after expiry
	carbon.SetTestNow(carbon.Now(carbon.UTC).AddMinutes(f.getMinutes()).AddSeconds(1)) // 1 second after expiry
	value, err = driver.Read(sessionID)
	f.Nil(err, "Read on expired session should not error")
	f.Equal("", value, "Session did not expire correctly")
	carbon.UnsetTestNow() // Clean up time travel
}

// TestWrite tests writing session data.
func (f *FileTestSuite) TestWrite() {
	driver := f.getDriver()
	f.Require().NotNil(driver)
	sessionID := "write_test_session"

	// 1. Initial write
	f.Nil(driver.Write(sessionID, "initial data"))
	value, err := driver.Read(sessionID)
	f.Nil(err)
	f.Equal("initial data", value)
	// Check file exists
	f.True(file.Exists(filepath.Join(f.getPath(), sessionID)))

	// 2. Overwrite
	f.Nil(driver.Write(sessionID, "overwritten data"))
	value, err = driver.Read(sessionID)
	f.Nil(err)
	f.Equal("overwritten data", value)
}

// BenchmarkFile_ReadWrite benchmarks combined read and write operations.
func BenchmarkFile_ReadWrite(b *testing.B) {
	// Setup outside the loop
	f := new(FileTestSuite)
	f.SetT(&testing.T{})
	f.BeforeTest("", "") // Run setup to ensure clean state

	// Use Require() from testify for setup, handled by getDriver
	driver := f.getDriver()
	require := f.Require() // Get require instance
	require.NotNil(driver)

	sessionID := "bench_session_id"
	sessionData := "benchmark data"

	// Initial write before timer starts
	err := driver.Write(sessionID, sessionData)
	require.Nil(err)

	b.ResetTimer() // Start timing benchmark iterations
	for i := 0; i < b.N; i++ {
		// Operation being benchmarked: Write then Read
		errWrite := driver.Write(sessionID, sessionData)
		if errWrite != nil { // Use standard error checking in benchmark loop for performance
			b.Fatalf("Write failed during benchmark: %v", errWrite)
		}

		value, errRead := driver.Read(sessionID)
		if errRead != nil {
			b.Fatalf("Read failed during benchmark: %v", errRead)
		}
		if value != sessionData { // Simple check instead of Equal for performance
			b.Fatalf("Read returned incorrect data during benchmark: got %s, want %s", value, sessionData)
		}
	}
	b.StopTimer() // Stop timing

	// Cleanup after benchmark (optional but good practice)
	// f.AfterTest("", "") // Use AfterTest for cleanup
}

// getDriver is a helper to create a File driver instance for tests.
// It uses the internal constructor newFile for isolation.
func (f *FileTestSuite) getDriver() sessioncontract.Driver {
	driver, err := newFile(f.getPath(), f.getMinutes()) // Use internal constructor
	// Use Require for setup failures - test cannot proceed without a driver.
	f.Require().NoError(err, "Failed to create file driver for test")
	f.Require().NotNil(driver, "Created driver is nil")
	return driver
}

// getPath returns the temporary directory used for test session files.
func (f *FileTestSuite) getPath() string {
	// Use a sub-directory within the project's temp/testing area if possible,
	// otherwise, a simple relative path is okay for local testing.
	return "storage/framework/sessions_test" // Example path
}

// getMinutes returns the session lifetime in minutes for testing.
func (f *FileTestSuite) getMinutes() int {
	return 5 // Use a shorter lifetime for quicker expiry tests
}
