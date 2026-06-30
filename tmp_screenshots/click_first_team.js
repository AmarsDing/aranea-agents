(() => {
  // Click the first team-card header to expand it
  const headers = document.querySelectorAll('.team-card__header');
  if (headers.length === 0) return 'No team-card headers found';
  headers[0].click();
  return `Clicked header ${0}, total headers: ${headers.length}`;
})()
