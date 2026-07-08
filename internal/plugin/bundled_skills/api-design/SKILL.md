---
name: api-design
description: RESTful API design patterns, endpoint naming, versioning, and error handling
version: "1.0.0"
author: graycode
license: MIT
category: engineering
tags: ["api", "rest", "design"]
allowed-tools: Read Write Grep
---

# API Design

## When to Use
- Designing new REST API endpoints
- Reviewing existing API for consistency
- Adding versioning or error handling

## Workflow
1. Use plural nouns for resources: `/users`, `/orders`
2. Use HTTP methods correctly: GET (read), POST (create), PUT (replace), PATCH (update), DELETE
3. Return appropriate status codes: 200, 201, 204, 400, 401, 403, 404, 409, 500
4. Use consistent error response format
5. Version via URL prefix: `/v1/users`
6. Support pagination: `?page=1&per_page=20`
7. Use HATEOAS links for discoverability

## Error Format
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Email is required",
    "details": [
      {"field": "email", "message": "must not be empty"}
    ]
  }
}
```

## Naming
- `GET /users` — list users
- `GET /users/:id` — get user
- `POST /users` — create user
- `PATCH /users/:id` — update user
- `DELETE /users/:id` — delete user
- `GET /users/:id/orders` — list user's orders

## Verification
- All endpoints follow RESTful naming conventions
- Error responses use consistent format
- Pagination is implemented for list endpoints
- Authentication/authorization is documented
