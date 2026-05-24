# API Layer Rules

- All endpoints must validate input with zod
- Return ApiResponse<T> from all handlers
- Never log raw request bodies
