package repomap

import (
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Go AST Parsing Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestParseGoAST_Functions(t *testing.T) {
	src := `package main

import "fmt"

func Hello() {
	fmt.Println("hello")
}

func Add(a, b int) int {
	return a + b
}

func unexported() {}
`
	symbols, err := parseGoAST("test.go", src, false)
	if err != nil {
		t.Fatal(err)
	}

	assertSymbol(t, symbols, "Hello", "func")
	assertSymbol(t, symbols, "Add", "func")
	assertNoSymbol(t, symbols, "unexported") // unexported, excluded by default
}

func TestParseGoAST_FunctionsIncludeUnexported(t *testing.T) {
	src := `package main

func Hello() {}
func unexported() {}
`
	symbols, err := parseGoAST("test.go", src, true)
	if err != nil {
		t.Fatal(err)
	}

	assertSymbol(t, symbols, "Hello", "func")
	assertSymbol(t, symbols, "unexported", "func")
}

func TestParseGoAST_Methods(t *testing.T) {
	src := `package svc

type Server struct {
	Port int
}

func (s *Server) Start() error {
	return nil
}

func (s *Server) Stop() {
}

func (s Server) Addr() string {
	return ""
}
`
	symbols, err := parseGoAST("svc.go", src, false)
	if err != nil {
		t.Fatal(err)
	}

	assertSymbol(t, symbols, "(*Server).Start", "method")
	assertSymbol(t, symbols, "(*Server).Stop", "method")
	assertSymbol(t, symbols, "(Server).Addr", "method")
	assertSymbol(t, symbols, "Server", "struct")
}

func TestParseGoAST_Types(t *testing.T) {
	src := `package types

type Config struct {
	Host string
	Port int
}

type Handler interface {
	ServeHTTP(w ResponseWriter, r *Request)
	Close() error
}

type StringAlias = string

type Mode int
`
	symbols, err := parseGoAST("types.go", src, false)
	if err != nil {
		t.Fatal(err)
	}

	assertSymbol(t, symbols, "Config", "struct")
	assertSymbol(t, symbols, "Handler", "interface")
	assertSymbol(t, symbols, "StringAlias", "type")
	assertSymbol(t, symbols, "Mode", "type")
}

func TestParseGoAST_InterfaceMethods(t *testing.T) {
	src := `package repo

type Repository interface {
	Get(id string) (Entity, error)
	Save(entity Entity) error
	Delete(id string) error
}
`
	symbols, err := parseGoAST("repo.go", src, false)
	if err != nil {
		t.Fatal(err)
	}

	assertSymbol(t, symbols, "Repository", "interface")
	assertSymbolContains(t, symbols, "Repository.Get()", "interface_method")
	assertSymbolContains(t, symbols, "Repository.Save()", "interface_method")
	assertSymbolContains(t, symbols, "Repository.Delete()", "interface_method")
}

func TestParseGoAST_StructFields(t *testing.T) {
	src := `package config

type AppConfig struct {
	Host     string
	Port     int
	Debug    bool
	internal string
}
`
	symbols, err := parseGoAST("config.go", src, false)
	if err != nil {
		t.Fatal(err)
	}

	// Exported fields should be included
	assertSymbolContains(t, symbols, "AppConfig.Host", "field")
	assertSymbolContains(t, symbols, "AppConfig.Port", "field")
	assertSymbolContains(t, symbols, "AppConfig.Debug", "field")

	// Unexported field should not be included
	for _, sym := range symbols {
		if strings.Contains(sym.Name, "internal") {
			t.Error("unexported field 'internal' should not be in symbols")
		}
	}
}

func TestParseGoAST_ConstsAndVars(t *testing.T) {
	src := `package constants

const (
	MaxRetries = 3
	Timeout    = 30
	internal   = "hidden"
)

var (
	ErrNotFound = errors.New("not found")
	errPrivate  = errors.New("private")
)
`
	symbols, err := parseGoAST("constants.go", src, false)
	if err != nil {
		t.Fatal(err)
	}

	assertSymbol(t, symbols, "MaxRetries", "const")
	assertSymbol(t, symbols, "Timeout", "const")
	assertSymbol(t, symbols, "ErrNotFound", "var")
	assertNoSymbol(t, symbols, "internal")
	assertNoSymbol(t, symbols, "errPrivate")
}

// ─────────────────────────────────────────────────────────────────────────────
// Python Enhanced Parsing Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestParsePythonEnhanced_ClassesWithMethods(t *testing.T) {
	src := `class AuthService:
    def __init__(self, db):
        self.db = db

    def validate_token(self, token: str) -> bool:
        pass

    def refresh_token(self, refresh: str) -> TokenPair:
        pass

class UserRepository:
    def find_by_id(self, user_id: int) -> User:
        pass
`
	symbols := parsePythonEnhanced(src)

	assertSymbol(t, symbols, "AuthService", "class")
	assertSymbol(t, symbols, "AuthService.__init__", "method")
	assertSymbol(t, symbols, "AuthService.validate_token", "method")
	assertSymbol(t, symbols, "AuthService.refresh_token", "method")
	assertSymbol(t, symbols, "UserRepository", "class")
	assertSymbol(t, symbols, "UserRepository.find_by_id", "method")
}

func TestParsePythonEnhanced_Inheritance(t *testing.T) {
	src := `class HTTPError(Exception):
    pass

class ValidationError(HTTPError, ValueError):
    def __init__(self, message):
        super().__init__(message)
`
	symbols := parsePythonEnhanced(src)

	assertSymbolKindContains(t, symbols, "HTTPError", "class(Exception)")
	assertSymbolKindContains(t, symbols, "ValidationError", "class(HTTPError, ValueError)")
}

func TestParsePythonEnhanced_Decorators(t *testing.T) {
	src := `@dataclass
class Config:
    host: str
    port: int

@app.route("/api")
def handle_request():
    pass

@staticmethod
def helper():
    pass
`
	symbols := parsePythonEnhanced(src)

	assertSymbolKindContains(t, symbols, "Config", "@dataclass")
	assertSymbolKindContains(t, symbols, "handle_request", "@app.route")
	assertSymbolKindContains(t, symbols, "helper", "@staticmethod")
}

func TestParsePythonEnhanced_AsyncFunctions(t *testing.T) {
	src := `async def fetch_data(url: str) -> dict:
    pass

class WebClient:
    async def get(self, url: str):
        pass

    async def post(self, url: str, data: dict):
        pass
`
	symbols := parsePythonEnhanced(src)

	assertSymbol(t, symbols, "fetch_data", "async func")
	assertSymbol(t, symbols, "WebClient", "class")
	assertSymbol(t, symbols, "WebClient.get", "async method")
	assertSymbol(t, symbols, "WebClient.post", "async method")
}

func TestParsePythonEnhanced_ModuleLevelFunctions(t *testing.T) {
	src := `def parse_config(path: str) -> Config:
    pass

def validate(data: dict) -> bool:
    pass

class Parser:
    def parse(self, input: str):
        pass

def standalone():
    pass
`
	symbols := parsePythonEnhanced(src)

	assertSymbol(t, symbols, "parse_config", "func")
	assertSymbol(t, symbols, "validate", "func")
	assertSymbol(t, symbols, "Parser.parse", "method")
	assertSymbol(t, symbols, "standalone", "func")
}

// ─────────────────────────────────────────────────────────────────────────────
// TypeScript/JavaScript Enhanced Parsing Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestParseTypeScriptEnhanced_Exports(t *testing.T) {
	src := `export function createServer(port: number): Server {
}

export class Router {
    private routes: Route[] = [];

    addRoute(path: string, handler: Handler): void {
    }

    match(url: string): Route | null {
    }
}

export interface Config {
    host: string;
    port: number;
}

export type RequestHandler = (req: Request) => Response;

export enum Status {
    Active,
    Inactive,
}
`
	symbols := parseTypeScriptEnhanced(src)

	assertSymbol(t, symbols, "createServer", "export func")
	assertSymbol(t, symbols, "Router", "export class")
	assertSymbol(t, symbols, "Router.addRoute", "method")
	assertSymbol(t, symbols, "Router.match", "method")
	assertSymbol(t, symbols, "Config", "export interface")
	assertSymbol(t, symbols, "RequestHandler", "export type")
	assertSymbol(t, symbols, "Status", "export enum")
}

func TestParseTypeScriptEnhanced_ArrowFunctions(t *testing.T) {
	src := `export const fetchUser = async (id: string): Promise<User> => {
};

export const validateEmail = (email: string): boolean => {
};

const internal = (x: number) => x * 2;
`
	symbols := parseTypeScriptEnhanced(src)

	assertSymbol(t, symbols, "fetchUser", "export func")
	assertSymbol(t, symbols, "validateEmail", "export func")
	assertSymbol(t, symbols, "internal", "func")
}

func TestParseTypeScriptEnhanced_ReactComponents(t *testing.T) {
	src := `export function UserProfile({ user }: Props) {
    return <div>{user.name}</div>;
}

export const Dashboard = ({ data }: DashboardProps) => {
    return <main>{data}</main>;
};

function HelperComponent() {
    return <span />;
}
`
	symbols := parseTypeScriptEnhanced(src)

	assertSymbolKindContains(t, symbols, "UserProfile", "component")
	assertSymbolKindContains(t, symbols, "Dashboard", "component")
	assertSymbolKindContains(t, symbols, "HelperComponent", "component")
}

func TestParseTypeScriptEnhanced_Interfaces(t *testing.T) {
	src := `export interface Repository<T> {
    findById(id: string): Promise<T>;
    save(entity: T): Promise<void>;
}

interface InternalCache {
    get(key: string): any;
    set(key: string, value: any): void;
}

export type Result<T> = { ok: true; data: T } | { ok: false; error: Error };
`
	symbols := parseTypeScriptEnhanced(src)

	assertSymbol(t, symbols, "Repository", "export interface")
	assertSymbol(t, symbols, "InternalCache", "interface")
	assertSymbol(t, symbols, "Result", "export type")
}

// ─────────────────────────────────────────────────────────────────────────────
// Rust Enhanced Parsing Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestParseRustEnhanced_PubItems(t *testing.T) {
	src := `pub fn process(input: &str) -> Result<Output, Error> {
}

fn internal_helper() {
}

pub struct Config {
    pub host: String,
    pub port: u16,
}

pub enum Status {
    Active,
    Inactive,
    Pending(String),
}

pub trait Handler {
    fn handle(&self, req: Request) -> Response;
}
`
	symbols := parseRustEnhanced(src)

	assertSymbol(t, symbols, "process", "pub fn")
	assertSymbol(t, symbols, "internal_helper", "fn")
	assertSymbol(t, symbols, "Config", "pub struct")
	assertSymbol(t, symbols, "Status", "pub enum")
	assertSymbol(t, symbols, "Handler", "pub trait")
}

func TestParseRustEnhanced_ImplBlocks(t *testing.T) {
	src := `pub struct Server {
    port: u16,
}

impl Server {
    pub fn new(port: u16) -> Self {
        Server { port }
    }

    pub fn start(&self) -> Result<(), Error> {
        Ok(())
    }

    fn internal(&self) {
    }
}

impl Display for Server {
    fn fmt(&self, f: &mut Formatter) -> fmt::Result {
        write!(f, "Server:{}", self.port)
    }
}
`
	symbols := parseRustEnhanced(src)

	assertSymbol(t, symbols, "Server", "pub struct")
	assertSymbol(t, symbols, "Server::new", "pub fn")
	assertSymbol(t, symbols, "Server::start", "pub fn")
	assertSymbol(t, symbols, "Server::internal", "fn")
	assertSymbolContains(t, symbols, "Display for Server", "impl")
}

func TestParseRustEnhanced_DeriveAttributes(t *testing.T) {
	src := `#[derive(Debug, Clone, Serialize)]
pub struct Config {
    pub host: String,
    pub port: u16,
}

#[derive(Debug, PartialEq)]
pub enum Mode {
    Development,
    Production,
}
`
	symbols := parseRustEnhanced(src)

	// Struct should have derive info in its kind
	found := false
	for _, sym := range symbols {
		if sym.Name == "Config" && strings.Contains(sym.Kind, "derive") {
			found = true
			if !strings.Contains(sym.Kind, "Debug") || !strings.Contains(sym.Kind, "Clone") || !strings.Contains(sym.Kind, "Serialize") {
				t.Errorf("Config derive should contain Debug, Clone, Serialize but got kind=%q", sym.Kind)
			}
		}
	}
	if !found {
		t.Error("Config struct with derive attributes not found")
	}

	// Enum should have derive info too
	found = false
	for _, sym := range symbols {
		if sym.Name == "Mode" && strings.Contains(sym.Kind, "derive") {
			found = true
			if !strings.Contains(sym.Kind, "Debug") || !strings.Contains(sym.Kind, "PartialEq") {
				t.Errorf("Mode derive should contain Debug, PartialEq but got kind=%q", sym.Kind)
			}
		}
	}
	if !found {
		t.Error("Mode enum with derive attributes not found")
	}
}

func TestParseRustEnhanced_AsyncAndUnsafe(t *testing.T) {
	src := `pub async fn fetch(url: &str) -> Result<Response, Error> {
}

pub unsafe fn raw_access(ptr: *mut u8) {
}

pub const fn max_size() -> usize {
    1024
}
`
	symbols := parseRustEnhanced(src)

	assertSymbol(t, symbols, "fetch", "async pub fn")
	assertSymbol(t, symbols, "raw_access", "unsafe pub fn")
	assertSymbol(t, symbols, "max_size", "const pub fn")
}

func TestParseRustEnhanced_Modules(t *testing.T) {
	src := `pub mod handlers;
mod internal;
pub mod models;
`
	symbols := parseRustEnhanced(src)

	assertSymbol(t, symbols, "handlers", "pub mod")
	assertSymbol(t, symbols, "internal", "mod")
	assertSymbol(t, symbols, "models", "pub mod")
}

// ─────────────────────────────────────────────────────────────────────────────
// TreeContext Rendering Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestRenderTreeContext_BasicOutput(t *testing.T) {
	symbols := []Symbol{
		{Name: "AuthService", Kind: "class", Line: 1},
		{Name: "AuthService.validate_token", Kind: "method", Line: 3},
		{Name: "AuthService.refresh_token", Kind: "method", Line: 6},
		{Name: "parse_config", Kind: "func", Line: 10},
	}

	output := RenderTreeContext("auth.py", symbols, 0)

	if !strings.Contains(output, "auth.py") {
		t.Error("output should contain filename")
	}
	if !strings.Contains(output, "AuthService") {
		t.Error("output should contain class name")
	}
	if !strings.Contains(output, "validate_token") {
		t.Error("output should contain method name")
	}
	if !strings.Contains(output, "refresh_token") {
		t.Error("output should contain method name")
	}
	if !strings.Contains(output, "parse_config") {
		t.Error("output should contain function name")
	}
}

func TestRenderTreeContext_Indentation(t *testing.T) {
	symbols := []Symbol{
		{Name: "Server", Kind: "struct", Line: 1},
		{Name: "Server.Start", Kind: "method", Line: 5},
		{Name: "Server.Stop", Kind: "method", Line: 10},
		{Name: "NewServer", Kind: "func", Line: 15},
	}

	output := RenderTreeContext("server.go", symbols, 0)
	lines := strings.Split(output, "\n")

	// The file name should be first
	if lines[0] != "server.go" {
		t.Errorf("first line should be filename, got: %q", lines[0])
	}

	// Parent should be indented with 2 spaces
	foundParent := false
	foundChild := false
	for _, line := range lines {
		if strings.Contains(line, "Server:") && strings.HasPrefix(line, "  ") {
			foundParent = true
		}
		if strings.Contains(line, "Start") && strings.HasPrefix(line, "    ") {
			foundChild = true
		}
	}
	if !foundParent {
		t.Error("parent scope should be indented with 2 spaces")
	}
	if !foundChild {
		t.Error("child symbols should be indented with 4 spaces")
	}
}

func TestRenderTreeContext_MaxLinesTruncation(t *testing.T) {
	symbols := []Symbol{
		{Name: "A", Kind: "class", Line: 1},
		{Name: "A.method1", Kind: "method", Line: 2},
		{Name: "A.method2", Kind: "method", Line: 3},
		{Name: "A.method3", Kind: "method", Line: 4},
		{Name: "A.method4", Kind: "method", Line: 5},
		{Name: "A.method5", Kind: "method", Line: 6},
	}

	output := RenderTreeContext("big.py", symbols, 4)

	// Should be truncated (file header + parent + 2 children max = 4 lines at the limit)
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) > 5 { // some tolerance for "..." line
		t.Errorf("expected truncation at ~4 lines, got %d lines:\n%s", len(lines), output)
	}
	if !strings.Contains(output, "...") {
		t.Error("truncated output should contain '...'")
	}
}

func TestRenderTreeContext_EmptySymbols(t *testing.T) {
	output := RenderTreeContext("empty.go", nil, 0)
	if output != "" {
		t.Errorf("expected empty output for nil symbols, got: %q", output)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration: ParseFileEnhanced / ParseSourceEnhanced
// ─────────────────────────────────────────────────────────────────────────────

func TestParseSourceEnhanced_GoDispatch(t *testing.T) {
	src := `package main

type Service struct {
	Name string
}

func (s *Service) Run() error {
	return nil
}

func NewService(name string) *Service {
	return &Service{Name: name}
}
`
	symbols, err := ParseSourceEnhanced(src, ".go")
	if err != nil {
		t.Fatal(err)
	}

	assertSymbol(t, symbols, "Service", "struct")
	assertSymbol(t, symbols, "(*Service).Run", "method")
	assertSymbol(t, symbols, "NewService", "func")
}

func TestParseSourceEnhanced_PythonDispatch(t *testing.T) {
	src := `class Database:
    def connect(self):
        pass

    async def query(self, sql: str):
        pass

def create_pool(size: int):
    pass
`
	symbols, err := ParseSourceEnhanced(src, ".py")
	if err != nil {
		t.Fatal(err)
	}

	assertSymbol(t, symbols, "Database", "class")
	assertSymbol(t, symbols, "Database.connect", "method")
	assertSymbol(t, symbols, "Database.query", "async method")
	assertSymbol(t, symbols, "create_pool", "func")
}

func TestParseSourceEnhanced_TypeScriptDispatch(t *testing.T) {
	src := `export interface Logger {
    info(msg: string): void;
    error(msg: string): void;
}

export const createLogger = (name: string): Logger => {
};

export class ConsoleLogger {
    info(msg: string): void {
    }
}
`
	symbols, err := ParseSourceEnhanced(src, ".ts")
	if err != nil {
		t.Fatal(err)
	}

	assertSymbol(t, symbols, "Logger", "export interface")
	assertSymbol(t, symbols, "createLogger", "export func")
	assertSymbol(t, symbols, "ConsoleLogger", "export class")
}

func TestParseSourceEnhanced_RustDispatch(t *testing.T) {
	src := `pub struct Engine {
    running: bool,
}

impl Engine {
    pub fn new() -> Self {
        Engine { running: false }
    }

    pub fn start(&mut self) {
        self.running = true;
    }
}

pub trait Runnable {
    fn run(&self);
}
`
	symbols, err := ParseSourceEnhanced(src, ".rs")
	if err != nil {
		t.Fatal(err)
	}

	assertSymbol(t, symbols, "Engine", "pub struct")
	assertSymbol(t, symbols, "Engine::new", "pub fn")
	assertSymbol(t, symbols, "Engine::start", "pub fn")
	assertSymbol(t, symbols, "Runnable", "pub trait")
}

// ─────────────────────────────────────────────────────────────────────────────
// TreeSitterParser struct tests
// ─────────────────────────────────────────────────────────────────────────────

func TestTreeSitterParser_IncludeUnexported(t *testing.T) {
	src := `package main

func Exported() {}
func unexported() {}
`
	p := &TreeSitterParser{IncludeUnexported: true}
	symbols, err := p.ParseSource(src, ".go", "test.go")
	if err != nil {
		t.Fatal(err)
	}

	assertSymbol(t, symbols, "Exported", "func")
	assertSymbol(t, symbols, "unexported", "func")
}

func TestTreeSitterParser_FallbackForUnsupported(t *testing.T) {
	src := `public class Main {
    public static void main(String[] args) {
    }
}
`
	p := NewTreeSitterParser()
	symbols, err := p.ParseSource(src, ".java", "Main.java")
	if err != nil {
		t.Fatal(err)
	}

	// Should fall back to Java regex parser
	assertSymbol(t, symbols, "Main", "class")
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// assertSymbol is defined in parser_langs_test.go (same package)
// ─────────────────────────────────────────────────────────────────────────────

func assertSymbolContains(t *testing.T, symbols []Symbol, nameSubstr, kind string) {
	t.Helper()
	for _, sym := range symbols {
		if strings.Contains(sym.Name, nameSubstr) && sym.Kind == kind {
			return
		}
	}
	t.Errorf("expected symbol containing Name=%q Kind=%q not found in:\n%s", nameSubstr, kind, fmtSymbolList(symbols))
}

func assertSymbolKindContains(t *testing.T, symbols []Symbol, name, kindSubstr string) {
	t.Helper()
	for _, sym := range symbols {
		if sym.Name == name && strings.Contains(sym.Kind, kindSubstr) {
			return
		}
	}
	t.Errorf("expected symbol Name=%q with Kind containing %q not found in:\n%s", name, kindSubstr, fmtSymbolList(symbols))
}

func assertNoSymbol(t *testing.T, symbols []Symbol, name string) {
	t.Helper()
	for _, sym := range symbols {
		if sym.Name == name {
			t.Errorf("unexpected symbol Name=%q found with Kind=%q", name, sym.Kind)
			return
		}
	}
}

func fmtSymbolList(symbols []Symbol) string {
	var b strings.Builder
	for _, sym := range symbols {
		b.WriteString("  " + sym.Kind + " " + sym.Name + "\n")
	}
	return b.String()
}
