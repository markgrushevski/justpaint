export type Tool = 'pen' | 'eraser' | 'line' | 'square' | 'triangle' | 'circle';
export type WorkHandler = 'undo' | 'redo' | 'save';
export type ToolbarItem = Tool & WorkHandler;
