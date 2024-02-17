export type ToolName = 'Pen' | 'Eraser' | 'Line' | 'Square' | 'Triangle' | 'Circle';
export type WorkHandlerName = 'Undo' | 'Redo' | 'Save';
export type ToolbarItem = ToolName & WorkHandlerName;
