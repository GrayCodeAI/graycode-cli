package tool

// This file holds TypeScript code-generation templates for the CodeGenerator.
// TypeScript templates are organized by category for maintainability.

func registerTSTemplates(cg *CodeGenerator) {
	// React Component
	cg.Templates["ts-react-component"] = &CodeTemplate{
		Name:        "ts-react-component",
		Description: "Functional React component with props interface",
		Language:    "typescript",
		Template: `import React from 'react';

interface {{.Name}}Props {
  title: string;
  className?: string;
  children?: React.ReactNode;
  onClick?: () => void;
}

/**
 * {{.Description}}
 */
export const {{.Name}}: React.FC<{{.Name}}Props> = ({
  title,
  className = '',
  children,
  onClick,
}) => {
  return (
    <div className={` + "`{{.Name}} ${className}`" + `} onClick={onClick}>
      <h2>{title}</h2>
      {children}
    </div>
  );
};

export default {{.Name}};
`,
		Variables: []TemplateVar{
			{Name: "Name", Description: "Component name (PascalCase)", Required: true, Default: ""},
			{Name: "Description", Description: "Component description", Required: false, Default: "A React component"},
		},
		Output: "{{.Name}}.tsx",
	}

	// Express Router
	cg.Templates["ts-express-router"] = &CodeTemplate{
		Name:        "ts-express-router",
		Description: "Express router with middleware",
		Language:    "typescript",
		Template: `import { Router, Request, Response, NextFunction } from 'express';

const router = Router();

// Middleware for this router
function validate{{.Name}}(req: Request, res: Response, next: NextFunction): void {
  // TODO: implement validation
  next();
}

// GET /{{.Path}}
router.get('/', async (req: Request, res: Response) => {
  try {
    // TODO: implement list
    res.json({ items: [] });
  } catch (error) {
    res.status(500).json({ error: 'Internal server error' });
  }
});

// GET /{{.Path}}/:id
router.get('/:id', async (req: Request, res: Response) => {
  try {
    const { id } = req.params;
    // TODO: implement get by id
    res.json({ id });
  } catch (error) {
    res.status(500).json({ error: 'Internal server error' });
  }
});

// POST /{{.Path}}
router.post('/', async (req: Request, res: Response) => {
  try {
    // TODO: implement create
    res.status(201).json({ id: 'new-id' });
  } catch (error) {
    res.status(500).json({ error: 'Internal server error' });
  }
});

// PUT /{{.Path}}/:id
router.put('/:id', async (req: Request, res: Response) => {
  try {
    const { id } = req.params;
    // TODO: implement update
    res.json({ id });
  } catch (error) {
    res.status(500).json({ error: 'Internal server error' });
  }
});

// DELETE /{{.Path}}/:id
router.delete('/:id', async (req: Request, res: Response) => {
  try {
    const { id } = req.params;
    // TODO: implement delete
    res.status(204).send();
  } catch (error) {
    res.status(500).json({ error: 'Internal server error' });
  }
});

export default router;
`,
		Variables: []TemplateVar{
			{Name: "Name", Description: "Router name (PascalCase)", Required: true, Default: ""},
			{Name: "Path", Description: "URL path prefix", Required: false, Default: "resources"},
		},
		Output: "{{.Name}}.router.ts",
	}

	// Test Describe Block
	cg.Templates["ts-test-describe"] = &CodeTemplate{
		Name:        "ts-test-describe",
		Description: "Jest/Vitest describe block with test cases",
		Language:    "typescript",
		Template: `import { describe, it, expect, beforeEach, afterEach } from 'vitest';

describe('{{.Name}}', () => {
  let subject: any;

  beforeEach(() => {
    // TODO: set up test fixtures
    subject = null;
  });

  afterEach(() => {
    // TODO: clean up
  });

  describe('{{.Method}}', () => {
    it('should handle valid input', () => {
      // Arrange
      const input = {};

      // Act
      const result = subject.{{.Method}}(input);

      // Assert
      expect(result).toBeDefined();
    });

    it('should throw on invalid input', () => {
      expect(() => subject.{{.Method}}(null)).toThrow();
    });

    it('should handle edge cases', () => {
      // TODO: implement edge case test
      expect(true).toBe(true);
    });
  });
});
`,
		Variables: []TemplateVar{
			{Name: "Name", Description: "Module/class under test", Required: true, Default: ""},
			{Name: "Method", Description: "Method being tested", Required: true, Default: "execute"},
		},
		Output: "{{.Name | lower}}.test.ts",
	}
}
