package integration

import (
	"context"
	"fmt"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Todo represents a todo item
type Todo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Completed   bool   `json:"completed"`
}

// TodoStore manages todo items in memory
type TodoStore struct {
	mu     sync.RWMutex
	todos  map[string]*Todo
	nextID int
}

// NewTodoStore creates a new todo store
func NewTodoStore() *TodoStore {
	return &TodoStore{
		todos:  make(map[string]*Todo),
		nextID: 1,
	}
}

// Add adds a new todo
func (s *TodoStore) Add(title, description string) *Todo {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("todo_%d", s.nextID)
	s.nextID++

	todo := &Todo{
		ID:          id,
		Title:       title,
		Description: description,
		Completed:   false,
	}
	s.todos[id] = todo
	return todo
}

// Get retrieves a todo by ID
func (s *TodoStore) Get(id string) (*Todo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	todo, ok := s.todos[id]
	if !ok {
		return nil, fmt.Errorf("todo not found: %s", id)
	}
	return todo, nil
}

// List returns all todos
func (s *TodoStore) List() []*Todo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Todo, 0, len(s.todos))
	for _, todo := range s.todos {
		result = append(result, todo)
	}
	return result
}

// Complete marks a todo as completed
func (s *TodoStore) Complete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	todo, ok := s.todos[id]
	if !ok {
		return fmt.Errorf("todo not found: %s", id)
	}
	todo.Completed = true
	return nil
}

// Delete removes a todo
func (s *TodoStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.todos[id]; !ok {
		return fmt.Errorf("todo not found: %s", id)
	}
	delete(s.todos, id)
	return nil
}

// CreateTodoServer creates an MCP server with todo app tools
func CreateTodoServer(hooks *server.Hooks) (*server.MCPServer, *TodoStore) {
	mcpServer := server.NewMCPServer(
		"todo-server",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithHooks(hooks),
	)

	store := NewTodoStore()

	// Add todo tool
	addTodoTool := mcp.NewTool(
		"add_todo",
		mcp.WithDescription("Add a new todo item"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Title of the todo"),
		),
		mcp.WithString("description",
			mcp.Description("Description of the todo"),
		),
	)

	mcpServer.AddTool(addTodoTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := request.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		description := request.GetString("description", "")

		todo := store.Add(title, description)
		return mcp.NewToolResultText(fmt.Sprintf("Added todo: %s (ID: %s)", todo.Title, todo.ID)), nil
	})

	// List todos tool
	listTodosTool := mcp.NewTool(
		"list_todos",
		mcp.WithDescription("List all todo items"),
	)

	mcpServer.AddTool(listTodosTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		todos := store.List()
		if len(todos) == 0 {
			return mcp.NewToolResultText("No todos found"), nil
		}

		result := "Todos:\n"
		for _, todo := range todos {
			status := "[ ]"
			if todo.Completed {
				status = "[x]"
			}
			result += fmt.Sprintf("%s %s - %s\n", status, todo.Title, todo.ID)
		}
		return mcp.NewToolResultText(result), nil
	})

	// Get todo tool
	getTodoTool := mcp.NewTool(
		"get_todo",
		mcp.WithDescription("Get a specific todo by ID"),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("ID of the todo"),
		),
	)

	mcpServer.AddTool(getTodoTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		todo, err := store.Get(id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		status := "incomplete"
		if todo.Completed {
			status = "complete"
		}
		return mcp.NewToolResultText(fmt.Sprintf("Todo: %s\nDescription: %s\nStatus: %s", todo.Title, todo.Description, status)), nil
	})

	// Complete todo tool
	completeTodoTool := mcp.NewTool(
		"complete_todo",
		mcp.WithDescription("Mark a todo as completed"),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("ID of the todo to complete"),
		),
	)

	mcpServer.AddTool(completeTodoTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		err = store.Complete(id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Completed todo: %s", id)), nil
	})

	// Delete todo tool
	deleteTodoTool := mcp.NewTool(
		"delete_todo",
		mcp.WithDescription("Delete a todo item"),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("ID of the todo to delete"),
		),
	)

	mcpServer.AddTool(deleteTodoTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		err = store.Delete(id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted todo: %s", id)), nil
	})

	return mcpServer, store
}
