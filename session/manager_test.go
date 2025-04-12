// path/to/goravel/framework/session/manager_test.go
package session

import (
	"fmt"
	"testing"

	// Needed for GC timer setup
	"github.com/stretchr/testify/mock" // Use testify mock
	"github.com/stretchr/testify/suite"

	// Import config contract
	"github.com/goravel/framework/contracts/foundation"
	sessioncontract "github.com/goravel/framework/contracts/session"
	"github.com/goravel/framework/errors"
	"github.com/goravel/framework/foundation/json"
	mockconfig "github.com/goravel/framework/mocks/config" // Use mock config
	// For potential future mocks if needed
)

// MockSessionDriver is a simple mock for testing purposes.
type MockSessionDriver struct {
	mock.Mock // Embed testify mock object
}

func (m *MockSessionDriver) Close() error {
	args := m.Called()
	return args.Error(0)
}
func (m *MockSessionDriver) Destroy(id string) error {
	args := m.Called(id)
	return args.Error(0)
}
func (m *MockSessionDriver) Gc(maxLifetime int) error {
	args := m.Called(maxLifetime)
	return args.Error(0)
}
func (m *MockSessionDriver) Open(path string, name string) error {
	args := m.Called(path, name)
	return args.Error(0)
}
func (m *MockSessionDriver) Read(id string) (string, error) {
	args := m.Called(id)
	return args.String(0), args.Error(1)
}
func (m *MockSessionDriver) Write(id string, data string) error {
	args := m.Called(id, data)
	return args.Error(0)
}

// MockDriverFactory creates an instance of the MockSessionDriver.
// Matches the signature 'func() (session.Driver, error)'.
func MockDriverFactory(mockDriverInstance *MockSessionDriver) func() (sessioncontract.Driver, error) {
	return func() (sessioncontract.Driver, error) {
		// In a real scenario, might return different instances or errors based on setup.
		if mockDriverInstance == nil {
			// Optionally create a default mock if none provided, or return error
			return nil, fmt.Errorf("mock driver instance not provided to factory")
		}
		return mockDriverInstance, nil // Return the shared mock instance or error if configured
	}
}

// CustomDriver for Extend tests
type CustomDriver struct{}

func NewCustomDriver() (sessioncontract.Driver, error) { // Match signature
	return &CustomDriver{}, nil
}
func (c *CustomDriver) Close() error                { return nil }
func (c *CustomDriver) Destroy(string) error        { return nil }
func (c *CustomDriver) Gc(int) error                { return nil }
func (c *CustomDriver) Open(string, string) error   { return nil }
func (c *CustomDriver) Read(string) (string, error) { return "", nil }
func (c *CustomDriver) Write(string, string) error  { return nil }

// --- Test Suite ---

type ManagerTestSuite struct {
	suite.Suite
	mockConfig        *mockconfig.Config
	manager           *Manager
	json              foundation.Json
	mockFileDriver    *MockSessionDriver // Instance of the mock driver for "file"
	mockFileDriverVia func() (sessioncontract.Driver, error)
}

func TestManagerTestSuite(t *testing.T) {
	suite.Run(t, &ManagerTestSuite{})
}

func (s *ManagerTestSuite) SetupTest() {
	s.mockConfig = mockconfig.NewConfig(s.T())
	s.json = json.NewJson()
	s.mockFileDriver = new(MockSessionDriver)                 // Create instance of the mock file driver
	s.mockFileDriverVia = MockDriverFactory(s.mockFileDriver) // Create factory func pointing to instance

	// --- Core Configuration for Manager Initialization ---
	// Mock the Get call for "session.drivers". This is CRITICAL for the new manager.
	// It tells the manager which drivers are available and how to create them.
	s.mockConfig.On("Get", "session.drivers", mock.AnythingOfType("map[string]interface {}")).Return(
		// Define the "file" driver using our mock factory
		map[string]any{
			"file": map[string]any{
				"via": s.mockFileDriverVia, // Point to the factory returning our mock driver
			},
			// Can add other pre-configured drivers for testing here if needed
		},
	).Maybe() // Maybe because not all tests might trigger config reading directly

	// --- Create Manager AFTER mocking essential config ---
	// NewManager will read the mocked "session.drivers" config now.
	s.manager = NewManager(s.mockConfig, s.json)
	s.Require().NotNil(s.manager)

	// // --- Other Common Mocks Needed by Methods Called Within Tests ---
	// // Mock GC interval used by startGcTimer (called within manager.Driver)
	// s.mockConfig.On("GetInt", "session.gc_interval").Return(30).Maybe()
	// // Mock Cookie name used by BuildSession
	// s.mockConfig.On("GetString", "session.cookie").Return("goravel_test_session").Maybe()
	// // Mock Default Driver name used by Driver() and getDefaultDriver()
	// s.mockConfig.On("GetString", "session.driver").Return("file").Maybe() // Default to "file"
}

// TearDownTest asserts mock expectations after each test.
func (s *ManagerTestSuite) TearDownTest() {
	s.mockConfig.AssertExpectations(s.T())
	// Assert mock driver expectations if needed
	// s.mockFileDriver.AssertExpectations(s.T())
}

func (s *ManagerTestSuite) TestDriver_ResolveConfiguredFileDriver() {
	// Manager is already initialized with "file" driver configured via mock factory in SetupTest

	s.mockConfig.On("GetString", "session.driver").Return("file").Maybe()
	s.mockConfig.On("GetInt", "session.gc_interval").Return(30).Maybe()

	// 1. Request driver by specific name "file"
	driver, err := s.manager.Driver("file")
	s.Nil(err)
	s.NotNil(driver)
	// Check if it's the specific mock instance we configured
	s.Equal(s.mockFileDriver, driver)
}

func (s *ManagerTestSuite) TestDriver_ResolveDefaultDriver() {
	// Manager is already initialized with "file" driver configured

	// Mock the config call to get the default driver *name*
	s.mockConfig.ExpectedCalls = nil // Clear previous maybe calls if strictness needed
	s.mockConfig.On("GetString", "session.driver").Return("file").Once()
	s.mockConfig.On("GetInt", "session.gc_interval").Return(30).Once() // Needed when Driver() instantiates

	// 2. Request default driver (no name provided)
	driver, err := s.manager.Driver()
	s.Nil(err)
	s.NotNil(driver)
	s.Equal(s.mockFileDriver, driver)
}

func (s *ManagerTestSuite) TestDriver_ResolveExtendedDriver() {

	s.mockConfig.On("GetString", "session.driver").Return("file").Maybe()
	s.mockConfig.On("GetInt", "session.gc_interval").Return(30).Maybe()
	// Extend with a custom driver
	err := s.manager.Extend("test", func() sessioncontract.Driver {
		d, _ := NewCustomDriver() // Assume factory doesn't error for this test
		return d
	})
	s.Nil(err)

	// Request the extended driver
	driver, err := s.manager.Driver("test")
	s.Nil(err)
	s.NotNil(driver)
	s.Equal("*session.CustomDriver", fmt.Sprintf("%T", driver)) // Check type
}

func (s *ManagerTestSuite) TestDriver_NotSupported() {
	// Mock config to return an unsupported driver name
	s.mockConfig.ExpectedCalls = nil
	s.mockConfig.On("GetString", "session.driver").Return("not_supported")
	// Ensure "session.drivers" mock (from setup) does NOT contain "not_supported"

	driver, _ := s.manager.Driver(s.manager.getDefaultDriver())
	s.manager.BuildSession(driver)

	// Request the unsupported driver
	driver, err := s.manager.Driver() // Gets name "not_supported" from mock
	s.NotNil(err)
	s.ErrorIs(err, errors.SessionDriverNotSupported)
	s.Equal(errors.SessionDriverNotSupported.Args("not_supported").Error(), err.Error())
	s.Nil(driver)
}

func (s *ManagerTestSuite) TestDriver_NotSet() {
	// Mock config to return an empty driver name
	s.mockConfig.ExpectedCalls = nil
	s.mockConfig.On("GetString", "session.driver").Return("").Once()

	// Request driver when default name is empty
	driver, err := s.manager.Driver()
	s.NotNil(err)
	s.ErrorIs(err, errors.SessionDriverIsNotSet)
	s.Nil(driver)
}

func (s *ManagerTestSuite) TestExtend() {

	s.mockConfig.On("GetString", "session.driver").Return("file").Maybe()
	s.mockConfig.On("GetInt", "session.gc_interval").Return(30).Maybe()

	// Extend should succeed
	err := s.manager.Extend("test", func() sessioncontract.Driver {
		d, _ := NewCustomDriver()
		return d
	})
	s.Nil(err)

	// Verify driver can be resolved
	driver, err := s.manager.Driver("test")
	s.Nil(err)
	s.NotNil(driver)
	s.Equal("*session.CustomDriver", fmt.Sprintf("%T", driver))
}

func (s *ManagerTestSuite) TestExtend_AlreadyExists() {
	// First extend (mock GC for later Driver call if we were to make one)
	// s.mockConfig.On("GetInt", "session.gc_interval", 30).Return(30).Once()
	err1 := s.manager.Extend("test", func() sessioncontract.Driver {
		d, _ := NewCustomDriver()
		return d
	})
	s.Nil(err1)

	// Second extend with the same name should fail
	err2 := s.manager.Extend("test", func() sessioncontract.Driver {
		d, _ := NewCustomDriver()
		return d
	})
	s.NotNil(err2)
	s.ErrorIs(err2, errors.SessionDriverAlreadyExists)
	s.EqualError(err2, errors.SessionDriverAlreadyExists.Args("test").Error())
}

func (s *ManagerTestSuite) TestBuildSession() {
	// Setup mocks needed for resolving the "file" driver and building session
	s.mockConfig.On("GetString", "session.cookie").Return("test_cookie").Once()
	s.mockConfig.On("GetString", "session.driver").Return("file").Maybe()
	s.mockConfig.On("GetInt", "session.gc_interval").Return(30).Maybe()

	// Resolve the driver (using the mocked factory)
	driver, err := s.manager.Driver(s.manager.getDefaultDriver())
	s.Nil(err)
	s.Require().NotNil(driver)        // Use require as session build depends on this
	s.Equal(s.mockFileDriver, driver) // Ensure it's our mock driver

	// Build the session
	session, err := s.manager.BuildSession(driver)
	s.Nil(err)
	s.Require().NotNil(session)

	// Use the session
	session.Put("name", "goravel")
	s.Equal("test_cookie", session.GetName())
	s.Equal("goravel", session.Get("name"))
	s.NotEmpty(session.GetID(), "Session ID should be generated or set")

	// Release the session and check reset
	s.manager.ReleaseSession(session)
	s.Empty(session.GetName(), "Session name should be empty after release")
	s.Empty(session.All(), "Session attributes should be empty after release")
}

func (s *ManagerTestSuite) TestBuildSession_NilDriver() {
	// Test BuildSession directly with nil driver
	session, err := s.manager.BuildSession(nil)
	s.ErrorIs(err, errors.SessionDriverIsNotSet)
	s.Nil(session)
}

func (s *ManagerTestSuite) TestGetDefaultDriver() {
	// Setup mock specific to this test
	s.mockConfig.ExpectedCalls = nil
	s.mockConfig.On("GetString", "session.driver").Return("custom_default").Once()

	// Test the private method directly (if necessary)
	s.Equal("custom_default", s.manager.getDefaultDriver())
}

// BenchmarkSession_ManagerInteraction benchmarks getting driver and building session
// It does NOT benchmark the underlying driver's read/write performance.
func BenchmarkSession_ManagerInteraction(b *testing.B) {
	s := new(ManagerTestSuite)
	s.SetT(&testing.T{})
	s.SetupTest() // Complete setup with mocks

	// Mocks needed repeatedly inside the loop
	s.mockConfig.On("GetString", "session.driver", "file").Return("file") // Called by Driver()
	s.mockConfig.On("GetInt", "session.gc_interval", 30).Return(30)       // Called by Driver() -> startGcTimer
	s.mockConfig.On("GetString", "session.cookie").Return("bench_cookie") // Called by BuildSession

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// --- Operations being benchmarked ---
		// 1. Resolve Driver
		driver, err := s.manager.Driver() // Uses default "file"
		if err != nil {
			b.Fatalf("Driver() failed during benchmark: %v", err)
		}
		if driver == nil {
			b.Fatal("Driver() returned nil driver during benchmark")
		}

		// 2. Build Session
		session, err := s.manager.BuildSession(driver)
		if err != nil {
			b.Fatalf("BuildSession() failed during benchmark: %v", err)
		}
		if session == nil {
			b.Fatal("BuildSession() returned nil session during benchmark")
		}

		// 3. Release Session
		s.manager.ReleaseSession(session)
		// --- End Operations ---
	}
	b.StopTimer()
}
