async (page) => {
  const session = await page.context().newCDPSession(page);
  const handleError = error => {
    if (!/Session closed|Target closed|has been closed/.test(error.message)) {
      throw error;
    }
  };
  session.on('Fetch.requestPaused', event => {
    session.send('Fetch.continueRequest', { requestId: event.requestId }).catch(handleError);
  });
  session.on('Fetch.authRequired', event => {
    session.send('Fetch.continueWithAuth', {
      requestId: event.requestId,
      authChallengeResponse: { response: 'CancelAuth' }
    }).catch(handleError);
  });
  await session.send('Fetch.enable', { handleAuthRequests: true });
  await page.reload();
}
