function defaultBackgroundFraming() {
  return {fit: 'fill', zoom: 100, positionX: 5000, positionY: 5000};
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
