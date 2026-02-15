ADK to Goose Coding Agent Specification (ADK2GOOSE)
1. Overview
This specification defines the requirements for building a Golang Wrapper Proxy that bridges an App Development Kit (ADK) with the Goose (Block) server via its REST API. The proxy acts as a translation layer, converting ADK-specific commands into Goose-compatible requests.

2. Architecture Components
• ADK Client Interface: Receives high-level intent or task definitions.

• Go Proxy Wrapper:

  • Request Transpiler: Maps ADK data structures to Goose REST payloads.

  • Authentication Handler: Manages API keys/OIDC tokens for the Goose server.

  • Session Manager: Tracks stateful interactions if required by the ADK.

• Goose Server: The target REST API (https://github.com/block/goose).

3. Technical Requirements
• Language: Go (Golang) using `go-sdk`.

• Communication:

  • Inbound: HTTP/JSON or Protobuf (depending on ADK).

  • Outbound: REST API calls to Goose.

• Streaming Support: Must implement SSE (Server-Sent Events) or WebSockets if the ADK requires real-time agent output.

• Concurrency: Utilize Go routines and `context.Context` for request cancellation and timeouts.

4. API Mapping Logic
5. Error Handling
• Translate Goose HTTP status codes (429, 500, etc.) into ADK-compliant error objects.

• Implement exponential backoff for rate-limited requests to the Goose backend.

6. Implementation Milestones
1. Client Setup: Initialize the Go HTTP client with Goose base URL and auth headers.

2. Model Definition: Define Go structs matching the Goose JSON schema.

3. Proxy Route: Implement the primary `ServeHTTP` handler to intercept ADK calls.

4. Integration Testing: Verify the bridge using a mock Goose server.
