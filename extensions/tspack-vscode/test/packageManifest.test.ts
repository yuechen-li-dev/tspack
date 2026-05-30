import * as fs from 'node:fs';
import * as path from 'node:path';
import { describe, expect, it } from 'vitest';

type PackageManifest = {
  contributes: {
    commands: Array<{ command: string; title: string }>;
    menus: {
      'view/item/context': Array<{
        command: string;
        when: string;
        group: string;
      }>;
    };
  };
};

const packageManifest = JSON.parse(
  fs.readFileSync(path.join(__dirname, '..', 'package.json'), 'utf8'),
) as PackageManifest;

describe('extension package manifest', () => {
  it('registers the reveal source command', () => {
    expect(packageManifest.contributes.commands).toContainEqual({
      command: 'tspack.inspect.revealSource',
      title: 'TSPack: Reveal Source for Selected Inspect Node',
    });
  });

  it('adds reveal source to the inspect-node-with-source context menu', () => {
    const contextMenus = packageManifest.contributes.menus['view/item/context'];

    expect(contextMenus).toContainEqual({
      command: 'tspack.inspect.revealSource',
      when: 'view == tspackInspectTree && viewItem == inspectNodeWithSource',
      group: 'inline',
    });
  });
});
