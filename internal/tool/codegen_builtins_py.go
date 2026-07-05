package tool

// This file holds Python code-generation templates for the CodeGenerator.
// Python templates are organized by category for maintainability.

func registerPyTemplates(cg *CodeGenerator) {
	// FastAPI Endpoint
	cg.Templates["py-fastapi-endpoint"] = &CodeTemplate{
		Name:        "py-fastapi-endpoint",
		Description: "FastAPI route with Pydantic model",
		Language:    "python",
		Template: `from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field
from typing import Optional

router = APIRouter(prefix="/{{.Prefix}}", tags=["{{.Tag}}"])


class {{.Name}}Request(BaseModel):
    """Request model for {{.Name}}."""
    name: str = Field(..., description="Name field")
    # TODO: add request fields


class {{.Name}}Response(BaseModel):
    """Response model for {{.Name}}."""
    id: str
    name: str
    # TODO: add response fields


@router.post("/", response_model={{.Name}}Response, status_code=201)
async def create_{{.NameLower}}(request: {{.Name}}Request) -> {{.Name}}Response:
    """Create a new {{.Name}}."""
    # TODO: implement creation logic
    return {{.Name}}Response(id="generated-id", name=request.name)


@router.get("/{item_id}", response_model={{.Name}}Response)
async def get_{{.NameLower}}(item_id: str) -> {{.Name}}Response:
    """Get a {{.Name}} by ID."""
    # TODO: implement retrieval logic
    raise HTTPException(status_code=404, detail="{{.Name}} not found")


@router.put("/{item_id}", response_model={{.Name}}Response)
async def update_{{.NameLower}}(item_id: str, request: {{.Name}}Request) -> {{.Name}}Response:
    """Update a {{.Name}}."""
    # TODO: implement update logic
    return {{.Name}}Response(id=item_id, name=request.name)


@router.delete("/{item_id}", status_code=204)
async def delete_{{.NameLower}}(item_id: str) -> None:
    """Delete a {{.Name}}."""
    # TODO: implement deletion logic
    pass
`,
		Variables: []TemplateVar{
			{Name: "Name", Description: "Resource name (PascalCase)", Required: true, Default: ""},
			{Name: "NameLower", Description: "Resource name (lowercase)", Required: true, Default: ""},
			{Name: "Prefix", Description: "URL prefix", Required: false, Default: "api"},
			{Name: "Tag", Description: "OpenAPI tag", Required: false, Default: "default"},
		},
		Output: "{{.NameLower}}_router.py",
	}

	// Pytest Test Class
	cg.Templates["py-test-class"] = &CodeTemplate{
		Name:        "py-test-class",
		Description: "Pytest test class with setup/teardown",
		Language:    "python",
		Template: `import pytest


class Test{{.Name}}:
    """Tests for {{.Name}}."""

    def setup_method(self):
        """Set up test fixtures."""
        # TODO: initialize test fixtures
        self.subject = None

    def teardown_method(self):
        """Clean up after tests."""
        # TODO: clean up resources
        pass

    def test_{{.MethodUnderTest}}_with_valid_input(self):
        """Test {{.MethodUnderTest}} with valid input."""
        # Arrange
        expected = None  # TODO: set expected value

        # Act
        result = self.subject.{{.MethodUnderTest}}()

        # Assert
        assert result == expected

    def test_{{.MethodUnderTest}}_with_invalid_input(self):
        """Test {{.MethodUnderTest}} raises on invalid input."""
        with pytest.raises(ValueError):
            self.subject.{{.MethodUnderTest}}(None)

    def test_{{.MethodUnderTest}}_edge_case(self):
        """Test {{.MethodUnderTest}} handles edge cases."""
        # TODO: implement edge case test
        pass
`,
		Variables: []TemplateVar{
			{Name: "Name", Description: "Class under test (PascalCase)", Required: true, Default: ""},
			{Name: "MethodUnderTest", Description: "Primary method to test", Required: true, Default: "execute"},
		},
		Output: "test_{{.Name | lower}}.py",
	}

	// Dataclass
	cg.Templates["py-dataclass"] = &CodeTemplate{
		Name:        "py-dataclass",
		Description: "Dataclass with validation",
		Language:    "python",
		Template: `from dataclasses import dataclass, field
from typing import Optional, List


@dataclass
class {{.Name}}:
    """{{.Description}}"""

    name: str
    value: int = 0
    tags: List[str] = field(default_factory=list)
    metadata: Optional[str] = None

    def __post_init__(self):
        """Validate fields after initialization."""
        if not self.name:
            raise ValueError("name must not be empty")
        if self.value < 0:
            raise ValueError("value must be non-negative")
        # TODO: add more validation

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "name": self.name,
            "value": self.value,
            "tags": list(self.tags),
            "metadata": self.metadata,
        }

    @classmethod
    def from_dict(cls, data: dict) -> "{{.Name}}":
        """Create instance from dictionary."""
        return cls(
            name=data["name"],
            value=data.get("value", 0),
            tags=data.get("tags", []),
            metadata=data.get("metadata"),
        )
`,
		Variables: []TemplateVar{
			{Name: "Name", Description: "Class name (PascalCase)", Required: true, Default: ""},
			{Name: "Description", Description: "Class description", Required: false, Default: "A data model"},
		},
		Output: "{{.Name | lower}}.py",
	}
}
