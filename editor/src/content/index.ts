export { compileTemplate } from "./parser.js";
export { DirectiveRegistry, createDefaultDirectiveRegistry } from "./directive-registry.js";
export type { DirectiveSignature, PositionalShape } from "./directive-registry.js";
export type {
  CompileResult,
  DirectiveArgs,
  DirectiveNode,
  GeometryArgs,
  SizeValue,
  TemplateCell,
  TemplateError,
  TemplateRow,
  TemplateTree,
} from "./template-types.js";
