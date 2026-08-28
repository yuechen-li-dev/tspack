import path from 'node:path';
import ts from 'typescript';

const SOURCE_ATTRIBUTE = 'data-tspack-source';
const COMPONENT_ATTRIBUTE = 'data-tspack-component';
const SYMBOL_ATTRIBUTE = 'data-tspack-symbol';
const TSPACK_ATTRIBUTES = new Set([
  SOURCE_ATTRIBUTE,
  COMPONENT_ATTRIBUTE,
  SYMBOL_ATTRIBUTE,
]);

export type InspectSourceInstrumentationOptions = {
  workspaceRoot: string;
  enabled: boolean;
};

export type InspectSourceInstrumentationResult = {
  code: string;
  map?: string;
  injectedNodeCount: number;
};

export type InspectSourceInstrumentation = {
  instrument(
    sourceCode: string,
    sourcePath: string,
  ): InspectSourceInstrumentationResult;
};

export type TspackVitePlugin = {
  name: string;
  enforce: 'pre';
  apply: 'serve';
  transform(
    sourceCode: string,
    sourcePath: string,
  ): InspectSourceInstrumentationResult | null;
};

function slashPath(value: string): string {
  return value.replace(/\\/g, '/');
}

function cleanVitePath(sourcePath: string): string {
  return sourcePath.split('?', 1)[0];
}

function isPascalCaseIdentifier(value: string): boolean {
  return /^[A-Z][A-Za-z0-9_$]*$/.test(value);
}

function componentNameForFunction(node: ts.Node): string | undefined {
  if (
    (ts.isFunctionDeclaration(node) ||
      ts.isFunctionExpression(node) ||
      ts.isMethodDeclaration(node)) &&
    node.name &&
    ts.isIdentifier(node.name)
  ) {
    return isPascalCaseIdentifier(node.name.text) ? node.name.text : undefined;
  }

  const parent = node.parent;
  if (
    parent &&
    ts.isVariableDeclaration(parent) &&
    ts.isIdentifier(parent.name) &&
    isPascalCaseIdentifier(parent.name.text)
  ) {
    return parent.name.text;
  }
  return undefined;
}

function intrinsicTagName(tagName: ts.JsxTagNameExpression): string | undefined {
  if (!ts.isIdentifier(tagName)) {
    return undefined;
  }
  const name = tagName.text;
  return /^[a-z]/.test(name) ? name : undefined;
}

function hasUserAuthoredTspackAttribute(attributes: ts.JsxAttributes): boolean {
  return attributes.properties.some((property) => {
    if (!ts.isJsxAttribute(property)) {
      return false;
    }
    return TSPACK_ATTRIBUTES.has(property.name.getText());
  });
}

function stringAttribute(name: string, value: string): ts.JsxAttribute {
  return ts.factory.createJsxAttribute(
    ts.factory.createIdentifier(name),
    ts.factory.createStringLiteral(value),
  );
}

function sourceLocation(sourceFile: ts.SourceFile, node: ts.Node, relativePath: string): string {
  const position = sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile));
  return `${relativePath}:${position.line + 1}:${position.character + 1}`;
}

function symbolForJsxNode(node: ts.Node): string | undefined {
  let current: ts.Node | undefined = node.parent;
  while (current && !ts.isFunctionLike(current)) {
    if (
      ts.isVariableDeclaration(current) &&
      ts.isIdentifier(current.name) &&
      isPascalCaseIdentifier(current.name.text)
    ) {
      return current.name.text;
    }
    current = current.parent;
  }
  return undefined;
}

function createInstrumentationTransformer(
  relativePath: string,
  onInjected: () => void,
): ts.TransformerFactory<ts.SourceFile> {
  return (context) => {
    let currentComponent: string | undefined;

    const visit: ts.Visitor = (node) => {
      if (
        ts.isFunctionDeclaration(node) ||
        ts.isFunctionExpression(node) ||
        ts.isArrowFunction(node) ||
        ts.isMethodDeclaration(node)
      ) {
        const previousComponent = currentComponent;
        currentComponent = componentNameForFunction(node) ?? currentComponent;
        const visited = ts.visitEachChild(node, visit, context);
        currentComponent = previousComponent;
        return visited;
      }

      if (ts.isJsxSelfClosingElement(node) || ts.isJsxOpeningElement(node)) {
        if (
          intrinsicTagName(node.tagName) &&
          !hasUserAuthoredTspackAttribute(node.attributes)
        ) {
          const properties = [...node.attributes.properties];
          properties.push(
            stringAttribute(
              SOURCE_ATTRIBUTE,
              sourceLocation(node.getSourceFile(), node, relativePath),
            ),
          );
          if (currentComponent) {
            properties.push(stringAttribute(COMPONENT_ATTRIBUTE, currentComponent));
          }
          const symbol = symbolForJsxNode(node);
          if (symbol && symbol !== currentComponent) {
            properties.push(stringAttribute(SYMBOL_ATTRIBUTE, symbol));
          }
          onInjected();

          const attributes = ts.factory.updateJsxAttributes(
            node.attributes,
            properties,
          );
          if (ts.isJsxSelfClosingElement(node)) {
            return ts.factory.updateJsxSelfClosingElement(
              node,
              node.tagName,
              node.typeArguments,
              attributes,
            );
          }
          return ts.factory.updateJsxOpeningElement(
            node,
            node.tagName,
            node.typeArguments,
            attributes,
          );
        }
      }

      return ts.visitEachChild(node, visit, context);
    };

    return (sourceFile) => ts.visitNode(sourceFile, visit) as ts.SourceFile;
  };
}

function isWorkspaceSource(workspaceRoot: string, sourcePath: string): boolean {
  const relative = path.relative(workspaceRoot, sourcePath);
  if (!relative || relative.startsWith('..') || path.isAbsolute(relative)) {
    return false;
  }
  return !slashPath(relative).split('/').includes('node_modules');
}

function supportsJsx(sourcePath: string): boolean {
  return /\.[jt]sx$/i.test(sourcePath);
}

export function createInspectSourceInstrumentation(
  options: InspectSourceInstrumentationOptions,
): InspectSourceInstrumentation {
  const workspaceRoot = path.resolve(options.workspaceRoot);

  return {
    instrument(sourceCode, sourcePath) {
      const absoluteSourcePath = path.resolve(cleanVitePath(sourcePath));
      if (
        !options.enabled ||
        !supportsJsx(absoluteSourcePath) ||
        !isWorkspaceSource(workspaceRoot, absoluteSourcePath)
      ) {
        return { code: sourceCode, injectedNodeCount: 0 };
      }

      const relativePath = slashPath(path.relative(workspaceRoot, absoluteSourcePath));
      let injectedNodeCount = 0;
      const result = ts.transpileModule(sourceCode, {
        fileName: relativePath,
        compilerOptions: {
          target: ts.ScriptTarget.ESNext,
          module: ts.ModuleKind.ESNext,
          jsx: ts.JsxEmit.Preserve,
          sourceMap: true,
          inlineSources: true,
          importsNotUsedAsValues: ts.ImportsNotUsedAsValues.Remove,
        },
        transformers: {
          before: [
            createInstrumentationTransformer(relativePath, () => {
              injectedNodeCount += 1;
            }),
          ],
        },
      });

      return {
        code: result.outputText,
        map: result.sourceMapText,
        injectedNodeCount,
      };
    },
  };
}

export function tspackInspectSourceVitePlugin(options: {
  workspaceRoot?: string;
  enabled?: boolean;
} = {}): TspackVitePlugin {
  const instrumentation = createInspectSourceInstrumentation({
    workspaceRoot: options.workspaceRoot ?? process.cwd(),
    enabled: options.enabled ?? true,
  });

  return {
    name: 'tspack-inspect-source-instrumentation',
    enforce: 'pre',
    apply: 'serve',
    transform(sourceCode, sourcePath) {
      const result = instrumentation.instrument(sourceCode, sourcePath);
      if (result.injectedNodeCount === 0) {
        return null;
      }
      return result;
    },
  };
}
