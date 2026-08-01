"use client";

import { useMemo } from "react";

type PassQrCodeProps = {
  value: string;
};

const QR_VERSION = 5;
const QR_SIZE = 17 + QR_VERSION * 4;
const QUIET_ZONE_SIZE = 4;
const DATA_CODEWORDS = 108;
const ERROR_CORRECTION_CODEWORDS = 26;
const MASK_PATTERN = 0;
const ERROR_CORRECTION_LEVEL_L = 1;
const MAX_BYTE_LENGTH = DATA_CODEWORDS - 2;

const EXPONENTIAL_TABLE = new Uint8Array(512);
const LOGARITHM_TABLE = new Uint8Array(256);

let fieldValue = 1;
for (let exponent = 0; exponent < 255; exponent += 1) {
  EXPONENTIAL_TABLE[exponent] = fieldValue;
  LOGARITHM_TABLE[fieldValue] = exponent;
  fieldValue <<= 1;
  if (fieldValue & 0x100) {
    fieldValue ^= 0x11d;
  }
}
for (let exponent = 255; exponent < EXPONENTIAL_TABLE.length; exponent += 1) {
  EXPONENTIAL_TABLE[exponent] = EXPONENTIAL_TABLE[exponent - 255];
}

function fieldExponent(value: number): number {
  return EXPONENTIAL_TABLE[value % 255];
}

function fieldLogarithm(value: number): number {
  if (value === 0) {
    throw new Error("Cannot compute a logarithm for zero.");
  }
  return LOGARITHM_TABLE[value];
}

function multiplyPolynomials(left: number[], right: number[]): number[] {
  const result = new Array<number>(left.length + right.length - 1).fill(0);
  for (let leftIndex = 0; leftIndex < left.length; leftIndex += 1) {
    for (let rightIndex = 0; rightIndex < right.length; rightIndex += 1) {
      if (left[leftIndex] === 0 || right[rightIndex] === 0) {
        continue;
      }
      result[leftIndex + rightIndex] ^=
        fieldExponent(fieldLogarithm(left[leftIndex]) + fieldLogarithm(right[rightIndex]));
    }
  }
  return result;
}

function errorCorrectionCodewords(data: Uint8Array): Uint8Array {
  let generator = [1];
  for (let index = 0; index < ERROR_CORRECTION_CODEWORDS; index += 1) {
    generator = multiplyPolynomials(generator, [1, fieldExponent(index)]);
  }

  const remainder = new Uint8Array(data.length + ERROR_CORRECTION_CODEWORDS);
  remainder.set(data);
  for (let index = 0; index < data.length; index += 1) {
    const coefficient = remainder[index];
    if (coefficient === 0) {
      continue;
    }
    const coefficientLogarithm = fieldLogarithm(coefficient);
    for (let generatorIndex = 0; generatorIndex < generator.length; generatorIndex += 1) {
      const generatorCoefficient = generator[generatorIndex];
      if (generatorCoefficient !== 0) {
        remainder[index + generatorIndex] ^=
          fieldExponent(fieldLogarithm(generatorCoefficient) + coefficientLogarithm);
      }
    }
  }

  return remainder.slice(data.length);
}

function pushBits(target: number[], value: number, length: number) {
  for (let bit = length - 1; bit >= 0; bit -= 1) {
    target.push((value >>> bit) & 1);
  }
}

function encodeData(value: string): Uint8Array | null {
  const data = new TextEncoder().encode(value);
  if (data.length > MAX_BYTE_LENGTH) {
    return null;
  }

  const bits: number[] = [];
  pushBits(bits, 0b0100, 4);
  pushBits(bits, data.length, 8);
  for (const byte of data) {
    pushBits(bits, byte, 8);
  }

  const capacity = DATA_CODEWORDS * 8;
  for (let index = 0; index < 4 && bits.length < capacity; index += 1) {
    bits.push(0);
  }
  while (bits.length % 8 !== 0) {
    bits.push(0);
  }

  const codewords: number[] = [];
  for (let index = 0; index < bits.length; index += 8) {
    let byte = 0;
    for (let bit = 0; bit < 8; bit += 1) {
      byte = (byte << 1) | bits[index + bit];
    }
    codewords.push(byte);
  }

  const padding = [0xec, 0x11];
  let paddingIndex = 0;
  while (codewords.length < DATA_CODEWORDS) {
    codewords.push(padding[paddingIndex % padding.length]);
    paddingIndex += 1;
  }

  const encodedData = Uint8Array.from(codewords);
  const correction = errorCorrectionCodewords(encodedData);
  const result = new Uint8Array(encodedData.length + correction.length);
  result.set(encodedData);
  result.set(correction, encodedData.length);
  return result;
}

function bchDigit(value: number): number {
  let digit = 0;
  let remaining = value;
  while (remaining !== 0) {
    digit += 1;
    remaining >>>= 1;
  }
  return digit;
}

function formatBits(maskPattern: number): number {
  const generator = 0b10100110111;
  const data = (ERROR_CORRECTION_LEVEL_L << 3) | maskPattern;
  let remainder = data << 10;
  while (bchDigit(remainder) >= bchDigit(generator)) {
    remainder ^= generator << (bchDigit(remainder) - bchDigit(generator));
  }
  return ((data << 10) | remainder) ^ 0b101010000010010;
}

function placeFinderPattern(modules: Array<Array<boolean | null>>, row: number, column: number) {
  for (let rowOffset = -1; rowOffset <= 7; rowOffset += 1) {
    const currentRow = row + rowOffset;
    if (currentRow < 0 || currentRow >= QR_SIZE) {
      continue;
    }
    for (let columnOffset = -1; columnOffset <= 7; columnOffset += 1) {
      const currentColumn = column + columnOffset;
      if (currentColumn < 0 || currentColumn >= QR_SIZE) {
        continue;
      }
      modules[currentRow][currentColumn] =
        (rowOffset >= 0 && rowOffset <= 6 && (columnOffset === 0 || columnOffset === 6)) ||
        (columnOffset >= 0 && columnOffset <= 6 && (rowOffset === 0 || rowOffset === 6)) ||
        (rowOffset >= 2 && rowOffset <= 4 && columnOffset >= 2 && columnOffset <= 4);
    }
  }
}

function placeAlignmentPatterns(modules: Array<Array<boolean | null>>) {
  const positions = [6, 30];
  for (const row of positions) {
    for (const column of positions) {
      if (modules[row][column] !== null) {
        continue;
      }
      for (let rowOffset = -2; rowOffset <= 2; rowOffset += 1) {
        for (let columnOffset = -2; columnOffset <= 2; columnOffset += 1) {
          modules[row + rowOffset][column + columnOffset] =
            rowOffset === -2 ||
            rowOffset === 2 ||
            columnOffset === -2 ||
            columnOffset === 2 ||
            (rowOffset === 0 && columnOffset === 0);
        }
      }
    }
  }
}

function placeTimingPatterns(modules: Array<Array<boolean | null>>) {
  for (let index = 8; index < QR_SIZE - 8; index += 1) {
    if (modules[index][6] === null) {
      modules[index][6] = index % 2 === 0;
    }
    if (modules[6][index] === null) {
      modules[6][index] = index % 2 === 0;
    }
  }
}

function placeFormatInformation(modules: Array<Array<boolean | null>>) {
  const bits = formatBits(MASK_PATTERN);
  for (let index = 0; index < 15; index += 1) {
    const dark = ((bits >>> index) & 1) === 1;
    if (index < 6) {
      modules[index][8] = dark;
    } else if (index < 8) {
      modules[index + 1][8] = dark;
    } else {
      modules[QR_SIZE - 15 + index][8] = dark;
    }

    if (index < 8) {
      modules[8][QR_SIZE - index - 1] = dark;
    } else if (index < 9) {
      modules[8][15 - index] = dark;
    } else {
      modules[8][15 - index - 1] = dark;
    }
  }
  modules[QR_SIZE - 8][8] = true;
}

function masked(row: number, column: number): boolean {
  return (row + column) % 2 === 0;
}

function placeData(modules: Array<Array<boolean | null>>, codewords: Uint8Array) {
  let row = QR_SIZE - 1;
  let direction = -1;
  let codewordIndex = 0;
  let bitIndex = 7;

  for (let column = QR_SIZE - 1; column > 0; column -= 2) {
    if (column === 6) {
      column -= 1;
    }

    while (true) {
      for (let offset = 0; offset < 2; offset += 1) {
        const currentColumn = column - offset;
        if (modules[row][currentColumn] !== null) {
          continue;
        }

        let dark = false;
        if (codewordIndex < codewords.length) {
          dark = ((codewords[codewordIndex] >>> bitIndex) & 1) === 1;
        }
        if (masked(row, currentColumn)) {
          dark = !dark;
        }
        modules[row][currentColumn] = dark;

        bitIndex -= 1;
        if (bitIndex < 0) {
          codewordIndex += 1;
          bitIndex = 7;
        }
      }

      row += direction;
      if (row < 0 || row >= QR_SIZE) {
        row -= direction;
        direction = -direction;
        break;
      }
    }
  }
}

function createMatrix(value: string): boolean[][] | null {
  const codewords = encodeData(value);
  if (!codewords) {
    return null;
  }

  const modules = Array.from({ length: QR_SIZE }, () =>
    new Array<boolean | null>(QR_SIZE).fill(null),
  );
  placeFinderPattern(modules, 0, 0);
  placeFinderPattern(modules, QR_SIZE - 7, 0);
  placeFinderPattern(modules, 0, QR_SIZE - 7);
  placeAlignmentPatterns(modules);
  placeTimingPatterns(modules);
  placeFormatInformation(modules);
  placeData(modules, codewords);

  return modules.map((row) => row.map((module) => module === true));
}

export function PassQrCode({ value }: PassQrCodeProps) {
  const matrix = useMemo(() => createMatrix(value), [value]);

  if (!matrix) {
    return (
      <p className="pass-qr-unavailable" role="alert">
        Your pass code could not be displayed. Ask an organizer to reissue your pass.
      </p>
    );
  }

  const modules = matrix.flatMap((row, rowIndex) =>
    row.flatMap((dark, columnIndex) =>
      dark ? `M${columnIndex} ${rowIndex}h1v1h-1z` : [],
    ),
  );

  return (
    <svg
      aria-label="QR code for your HackAtlantic entry pass"
      className="pass-qr-code"
      role="img"
      shapeRendering="crispEdges"
      viewBox={`0 0 ${QR_SIZE + QUIET_ZONE_SIZE * 2} ${QR_SIZE + QUIET_ZONE_SIZE * 2}`}
      xmlns="http://www.w3.org/2000/svg"
    >
      <rect
        fill="#ffffff"
        height={QR_SIZE + QUIET_ZONE_SIZE * 2}
        width={QR_SIZE + QUIET_ZONE_SIZE * 2}
        x="0"
        y="0"
      />
      <path
        d={modules.join("")}
        fill="#111827"
        transform={`translate(${QUIET_ZONE_SIZE} ${QUIET_ZONE_SIZE})`}
      />
    </svg>
  );
}
