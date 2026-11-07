const assert = require('node:assert/strict');
const test = require('node:test');
const { Terminal } = require('@xterm/headless');

function write(terminal, output) {
  return new Promise(resolve => terminal.write(output, resolve));
}

function color(cell, foreground) {
  if (foreground ? cell.isFgDefault() : cell.isBgDefault()) return null;
  const value = foreground ? cell.getFgColor() : cell.getBgColor();
  const rgb = foreground ? cell.isFgRGB() : cell.isBgRGB();
  return rgb ? [value >>> 16, (value >>> 8) & 255, value & 255] : { palette: value };
}

function snapshot(terminal) {
  const buffer = terminal.buffer.active;
  const core = terminal._core;
  // Headless's public API omits OSC 8 targets and cursor visibility. The pinned
  // emulator owns those facts; an upstream contract change must fail this test.
  assert.equal(typeof core._oscLinkService.getLinkData, 'function');
  assert.equal(typeof core.coreService.isCursorHidden, 'boolean');
  const rows = Array.from({ length: terminal.rows }, (_, y) => {
    const line = buffer.getLine(buffer.baseY + y);
    return Array.from({ length: terminal.cols }, (_, x) => {
      const cell = line.getCell(x);
      let link = '';
      assert.equal(typeof cell.extended.urlId, 'number');
      if (cell.extended.urlId) {
        const data = core._oscLinkService.getLinkData(cell.extended.urlId);
        assert.ok(data, `unresolved hyperlink at ${x},${y}`);
        link = data.uri;
      }
      return {
        text: cell.getChars() || (cell.getWidth() ? ' ' : ''),
        columns: cell.getWidth(),
        style: {
          foreground: color(cell, true), background: color(cell, false),
          bold: !!cell.isBold(), dim: !!cell.isDim(), italic: !!cell.isItalic(),
          underline: !!cell.isUnderline(), reverse: !!cell.isInverse(), strike: !!cell.isStrikethrough(),
        },
        link,
      };
    });
  });
  const cursor = { visible: !core.coreService.isCursorHidden };
  if (cursor.visible) {
    Object.assign(cursor, {
      x: buffer.cursorX, y: buffer.cursorY,
      shape: terminal.options.cursorStyle, blink: terminal.options.cursorBlink,
    });
  }
  return {
    rows, cursor, buffer: buffer.type,
    margins: [core._bufferService.buffer.scrollTop, core._bufferService.buffer.scrollBottom],
  };
}

function expected(frame) {
  const rows = frame.rows.map(row => row.flatMap(atom => {
    const style = { ...atom.style, underline: atom.style.underline || !!atom.link };
    const cells = [{ ...atom, style }];
    for (let column = 1; column < atom.columns; column++) {
      cells.push({ text: '', columns: 0, style, link: atom.link });
    }
    return cells;
  }));
  assert.equal(rows.length, frame.size.rows, 'fixture row count');
  for (const row of rows) assert.equal(row.length, frame.size.columns, 'fixture column count');
  const cursor = frame.cursor.visible ? frame.cursor : { visible: false };
  return { rows, cursor, buffer: 'alternate', margins: [0, frame.size.rows - 1] };
}

module.exports = function registerSequences(sequences) {
  for (const sequence of sequences) {
    test(sequence.name, async () => {
      const size = sequence.frames[0].size;
      const terminals = Array.from({ length: 2 }, () => new Terminal({
        cols: size.columns, rows: size.rows, allowProposedApi: true,
      }));
      try {
        for (const terminal of terminals) await write(terminal, '\x1b[?1049h');
        for (const frame of sequence.frames) {
          const label = `${sequence.name}, ${frame.name}`;
          if (frame.unchanged) assert.equal(frame.diff, '', `${label}: idle frame writes nothing`);
          if (frame.scroll) assert.match(frame.diff, /\x1b\[\d*[ST]/, `${label}: exercise the scroll optimization`);
          const wanted = expected(frame);
          for (let index = 0; index < terminals.length; index++) {
            const terminal = terminals[index];
            terminal.resize(frame.size.columns, frame.size.rows);
            const output = index === 0 ? frame.diff : frame.full;
            if (output) await write(terminal, output);
            assert.deepEqual(snapshot(terminal), wanted, `${label}: ${index === 0 ? 'diff' : 'full'} matches independent cell model`);
          }
          assert.deepEqual(snapshot(terminals[0]), snapshot(terminals[1]), `${label}: diff equals full`);
        }
      } finally {
        for (const terminal of terminals) terminal.dispose();
      }
    });
  }
};
