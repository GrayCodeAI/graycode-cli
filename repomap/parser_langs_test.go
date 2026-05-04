package repomap

import "testing"

func assertSymbol(t *testing.T, symbols []Symbol, name, kind string) {
	t.Helper()
	for _, s := range symbols {
		if s.Name == name && s.Kind == kind {
			return
		}
	}
	t.Errorf("expected symbol %s (%s), got %v", name, kind, symbols)
}

func TestParseC(t *testing.T) {
	src := `#define MAX_SIZE 100
struct Node {
    int val;
};
enum Color { RED, GREEN };
int main(int argc, char **argv) {
    return 0;
}
static void helper(void) {}
`
	syms := parseC(src)
	assertSymbol(t, syms, "MAX_SIZE", "define")
	assertSymbol(t, syms, "Node", "struct")
	assertSymbol(t, syms, "Color", "enum")
	assertSymbol(t, syms, "main", "func")
	assertSymbol(t, syms, "helper", "func")
}

func TestParseCpp(t *testing.T) {
	src := `namespace mylib {
class Widget {
public:
    void draw();
};
template<typename T>
class Container {};
}
`
	syms := parseCpp(src)
	assertSymbol(t, syms, "mylib", "namespace")
	assertSymbol(t, syms, "Widget", "class")
	assertSymbol(t, syms, "Container", "class")
}

func TestParseCSharp(t *testing.T) {
	src := `namespace MyApp.Models;
public class User {
    public string Name { get; set; }
    public async Task<bool> SaveAsync(string path) { }
}
public interface IRepository { }
public record Point(int X, int Y);
public enum Status { Active, Inactive }
`
	syms := parseCSharp(src)
	assertSymbol(t, syms, "MyApp.Models", "namespace")
	assertSymbol(t, syms, "User", "class")
	assertSymbol(t, syms, "SaveAsync", "method")
	assertSymbol(t, syms, "IRepository", "interface")
	assertSymbol(t, syms, "Point", "record")
	assertSymbol(t, syms, "Status", "enum")
}

func TestParsePHP(t *testing.T) {
	src := `<?php
abstract class Controller {
    public function index() {}
    private static function helper() {}
}
interface Renderable {}
trait Cacheable {}
function globalHelper() {}
`
	syms := parsePHP(src)
	assertSymbol(t, syms, "Controller", "class")
	assertSymbol(t, syms, "index", "func")
	assertSymbol(t, syms, "helper", "func")
	assertSymbol(t, syms, "Renderable", "interface")
	assertSymbol(t, syms, "Cacheable", "trait")
	assertSymbol(t, syms, "globalHelper", "func")
}

func TestParseRuby(t *testing.T) {
	src := `module Auth
class User
  def initialize(name)
  end
  def self.find(id)
  end
  def valid?
  end
end
end
`
	syms := parseRuby(src)
	assertSymbol(t, syms, "Auth", "module")
	assertSymbol(t, syms, "User", "class")
	assertSymbol(t, syms, "initialize", "func")
	assertSymbol(t, syms, "self.find", "func")
	assertSymbol(t, syms, "valid?", "func")
}

func TestParseKotlin(t *testing.T) {
	src := `data class User(val name: String)
sealed class Result
interface Repository
object Singleton
enum class Color { RED, GREEN }
suspend fun fetchData(): List<String> {}
fun process(items: List<Int>) {}
`
	syms := parseKotlin(src)
	assertSymbol(t, syms, "User", "data class")
	assertSymbol(t, syms, "Result", "class")
	assertSymbol(t, syms, "Repository", "interface")
	assertSymbol(t, syms, "Singleton", "object")
	assertSymbol(t, syms, "Color", "enum")
	assertSymbol(t, syms, "fetchData", "func")
	assertSymbol(t, syms, "process", "func")
}

func TestParseSwift(t *testing.T) {
	src := `public class ViewController {
    func viewDidLoad() {}
    static func create() -> Self {}
}
struct Point { var x: Int; var y: Int }
enum Direction { case north, south }
protocol Drawable { func draw() }
`
	syms := parseSwift(src)
	assertSymbol(t, syms, "ViewController", "class")
	assertSymbol(t, syms, "viewDidLoad", "func")
	assertSymbol(t, syms, "create", "func")
	assertSymbol(t, syms, "Point", "struct")
	assertSymbol(t, syms, "Direction", "enum")
	assertSymbol(t, syms, "Drawable", "protocol")
	// draw() is inside protocol body — Swift parser extracts top-level declarations
}

func TestParseScala(t *testing.T) {
	src := `trait Service {
  def process(input: String): Future[Result]
}
case class User(name: String, age: Int)
object UserService {
  def findById(id: Long): Option[User] = ???
}
`
	syms := parseScala(src)
	assertSymbol(t, syms, "Service", "trait")
	assertSymbol(t, syms, "process", "func")
	assertSymbol(t, syms, "User", "case class")
	assertSymbol(t, syms, "UserService", "object")
	assertSymbol(t, syms, "findById", "func")
}

func TestParseLua(t *testing.T) {
	src := `function M.setup(opts)
end
local function helper()
end
M.callback = function(event)
end
`
	syms := parseLua(src)
	assertSymbol(t, syms, "M.setup", "func")
	assertSymbol(t, syms, "helper", "func")
	assertSymbol(t, syms, "M.callback", "func")
}

func TestParseDart(t *testing.T) {
	src := `abstract class Widget {
  void build(BuildContext context);
}
class MyApp extends StatelessWidget {}
enum AppState { loading, ready, error }
Future<void> main() async {}
`
	syms := parseDart(src)
	assertSymbol(t, syms, "Widget", "class")
	assertSymbol(t, syms, "MyApp", "class")
	assertSymbol(t, syms, "AppState", "enum")
	assertSymbol(t, syms, "main", "func")
}

func TestParseElixir(t *testing.T) {
	src := `defmodule MyApp.Accounts do
  def create_user(attrs) do
  end
  defp validate(attrs) do
  end
  defmacro is_admin?(user) do
  end
end
`
	syms := parseElixir(src)
	assertSymbol(t, syms, "MyApp.Accounts", "module")
	assertSymbol(t, syms, "create_user", "func")
	assertSymbol(t, syms, "validate", "func")
	assertSymbol(t, syms, "is_admin?", "func")
}

func TestParseHaskell(t *testing.T) {
	src := `data Tree a = Leaf | Node (Tree a) a (Tree a)
newtype Wrapper a = Wrapper { unwrap :: a }
class Functor f where
  fmap :: (a -> b) -> f a -> f b
insert :: Ord a => a -> Tree a -> Tree a
insert x Leaf = Node Leaf x Leaf
`
	syms := parseHaskell(src)
	assertSymbol(t, syms, "Tree", "type")
	assertSymbol(t, syms, "Wrapper", "type")
	assertSymbol(t, syms, "Functor", "class")
	assertSymbol(t, syms, "insert", "func")
}

// Test improved existing parsers

func TestParseJavaImproved(t *testing.T) {
	src := `public class UserService {
    public static final String TABLE = "users";
    public User findById(long id) { return null; }
    private void validate(User u) {}
}
public record Point(int x, int y) {}
public interface Repository<T> {}
`
	syms := parseJava(src)
	assertSymbol(t, syms, "UserService", "class")
	assertSymbol(t, syms, "findById", "method")
	assertSymbol(t, syms, "Point", "record")
	assertSymbol(t, syms, "Repository", "interface")
}

func TestParsePythonDecorators(t *testing.T) {
	src := `@app.route("/")
def index():
    pass

@staticmethod
def helper():
    pass

class MyClass:
    pass
`
	syms := parsePython(src)
	assertSymbol(t, syms, "index", "@app.route func")
	assertSymbol(t, syms, "helper", "@staticmethod func")
	assertSymbol(t, syms, "MyClass", "class")
}

func TestParseTSArrowFunctions(t *testing.T) {
	src := `export const fetchUsers = async (limit: number) => {}
export function processData(input: string): void {}
export enum Status { Active, Inactive }
export abstract class BaseService {}
const helper = (x: number) => x * 2
`
	syms := parseTypeScript(src)
	assertSymbol(t, syms, "fetchUsers", "func")
	assertSymbol(t, syms, "processData", "func")
	assertSymbol(t, syms, "Status", "enum")
	assertSymbol(t, syms, "BaseService", "abstract class")
	assertSymbol(t, syms, "helper", "func")
}
