# Mocker - Java JAX-RS Controller Parser

A Go CLI tool that parses Java JAX-RS controller files using tree-sitter AST parsing and extracts endpoint information with fully qualified return types and deep type details.

## Installation

```bash
go build -o mocker ./cmd/main.go
```

## Usage

```bash
./mocker [-src <source-directory>] <path-to-java-controller-file>
```

### Arguments

- `path-to-java-controller-file` (required): Path to the Java controller file

### Flags

- `-src` (optional): Source directory to scan for imported files (default: current directory ".")

## Output

The tool generates an `endpoints.json` file in the current directory with the following structure:

```json
{
  "filename": "UserController.java",
  "basePath": "/api/users",
  "produces": ["MediaType.APPLICATION_JSON"],
  "consumes": ["MediaType.APPLICATION_JSON"],
  "endpoints": [
    {
      "method": "GET",
      "path": "/api/users/{id}",
      "returnType": "com.example.User",
      "returnTypes": ["com.example.User"],
      "returnTypeDetails": [{"name": "User", "fields": [...]}],
      "typeDetails": {
        "package": "com.example",
        "name": "User",
        "fullName": "com.example.User",
        "fields": [
          {
            "name": "id",
            "type": "java.lang.Long",
            "isCollection": false
          },
          {
            "name": "name",
            "type": "java.lang.String",
            "isCollection": false
          },
          {
            "name": "address",
            "type": "com.example.Address",
            "typeDetails": {
              "package": "com.example",
              "name": "Address",
              "fullName": "com.example.Address",
              "fields": [
                {"name": "street", "type": "java.lang.String"},
                {"name": "city", "type": "java.lang.String"}
              ]
            },
            "isCollection": false
          },
          {
            "name": "orders",
            "type": "java.util.List<com.example.Order>",
            "typeDetails": {
              "package": "com.example",
              "name": "Order",
              "fields": [...]
            },
            "isCollection": true,
            "genericArgs": ["com.example.Order"]
          }
        ]
      },
      "handler": "getUserById",
      "consumes": ["MediaType.APPLICATION_JSON"],
      "produces": ["MediaType.APPLICATION_JSON"]
    }
  ]
}
```

## Features

### JAX-RS Annotation Support
- `@Path` (class and method level)
- `@GET`, `@POST`, `@PUT`, `@DELETE`, `@PATCH`, `@HEAD`, `@OPTIONS`
- `@Produces`, `@Consumes`

### Response Type Resolution
Methods returning `jakarta.ws.rs.core.Response` are automatically parsed to extract the actual entity type from the method body:

```java
@POST
public Response createUser(User user) {
    User created = userService.save(user);
    return Response.ok(created).build();  // Returns: com.example.User
}

@DELETE
@Path("/{id}")
public Response deleteUser(@PathParam("id") Long id) {
    userService.delete(id);
    return Response.noContent().build();  // Returns: void
}

@POST
@Path("/{id}/orders")
public Response createOrder(@PathParam("id") Long id, Order order) {
    Order created = orderService.create(id, order);
    return Response.status(201).entity(created).build();  // Returns: com.example.Order
}
```

Supported Response patterns:
- `Response.ok(entity).build()` → extracts entity type
- `Response.noContent().build()` → void
- `Response.status(code).entity(entity).build()` → extracts entity type
- `Response.status(code).build()` → void
- `Response.accepted(entity).build()` → extracts entity type
- `Response.created(uri).entity(entity).build()` → extracts entity type

### Type Resolution
- **Imported types**: Resolves from import statements (e.g., `List<User>` → `java.util.List<com.example.User>`)
- **java.lang types**: Automatically qualified (e.g., `String` → `java.lang.String`)
- **Local classes**: Scans same package for unresolved types
- **Collection types**: Properly handles `List<T>`, `Set<T>`, `Map<K,V>`, etc.
- **Multiple return types**: For methods with conditional returns (e.g., `if (x) return A; else return B;`), all possible types are included in `returnTypes`
- **Type details array**: `returnTypeDetails` provides full type information for each return type (parallel to `returnTypes`, with null for void/primitives)
- **Generic type resolution**: For collection types like `List<User>`, type details are resolved for the generic type argument (User)

### Deep Type Details
- Follows nested objects recursively
- Resolves generic type arguments
- Includes inheritance (scans parent class fields)
- Maximum recursion depth: 10 levels

## Example

Given a controller:

```java
package com.example;

import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import java.util.List;

@Path("/users")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
public class UserController {

    @GET
    @Path("/{id}")
    public User getUserById(@PathParam("id") Long id) {
        return null;
    }

    @GET
    @Path("/")
    public List<User> getUsers() {
        return null;
    }
}
```

And a `User.java` in the same package:

```java
package com.example;

public class User {
    private Long id;
    private String name;
    private Address address;
    private List<Order> orders;
}
```

Run:

```bash
./mocker -src . com/example/UserController.java
```

Output: See the JSON structure above with full type details for `User`, `Address`, `Order`, etc.

## Building

```bash
# Build the CLI
go build -o mocker ./cmd/main.go

# Or install globally
go install ./cmd/main.go
```

## Requirements

- Go 1.25+
- tree-sitter Java grammar
