function defaultBackgroundFraming() {
  return {fit: 'fill', zoom: 100, positionX: 5000, positionY: 5000};
}
// Match the native stored-image budget so preview framing uses the same aspect
// ratio even when flooring a resized, very thin image changes it slightly.
function backgroundImportDimensions(width, height) {
  // 3072 limits each axis; 4,000,000 limits area. Area changes as scale²,
  // so its constraint uses a square root. The leading 1 prevents upscaling.
  const scale = Math.min(1, 3072 / width, 3072 / height, Math.sqrt(4000000 / (width * height)));
  return {width: Math.max(1, Math.floor(width * scale)), height: Math.max(1, Math.floor(height * scale))};
}
function safeBackgroundFraming(value) {
  if (!value || !['fill', 'fit'].includes(value.fit) ||
      !Number.isInteger(value.zoom) || value.zoom < 100 || value.zoom > 300 ||
      !Number.isInteger(value.positionX) || value.positionX < 0 || value.positionX > 10000 ||
      !Number.isInteger(value.positionY) || value.positionY < 0 || value.positionY > 10000) return defaultBackgroundFraming();
  return {...value};
}
function backgroundGeometry(imageWidth, imageHeight, viewportWidth, viewportHeight, framing) {
  const fit = framing.fit === 'fit';
  // Positions describe fractions of the overflow, not source-image pixels.
  // The identical calculation at preview scale preserves the viewport crop.
  const scale = (fit ? Math.min : Math.max)(viewportWidth / imageWidth, viewportHeight / imageHeight) * (fit ? 1 : framing.zoom / 100);
  const width = imageWidth * scale;
  const height = imageHeight * scale;
  return {
    width, height,
    left: (viewportWidth - width) * (fit ? 0.5 : framing.positionX / 10000),
    top: (viewportHeight - height) * (fit ? 0.5 : framing.positionY / 10000)
  };
}
