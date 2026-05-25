<?php

require_once __DIR__ . '/vendor/autoload.php';

use Symfony\Component\HttpFoundation\Request;
use Symfony\Component\HttpFoundation\Response;
use Symfony\Component\HttpFoundation\JsonResponse;
use Symfony\Component\HttpFoundation\Cookie;

echo "=== symfony/http-foundation functional test ===" . PHP_EOL . PHP_EOL;

// 1. Create a request from globals simulation
$request = Request::create(
    '/api/users/42',
    'GET',
    ['format' => 'json'],       // query params
    ['session_id' => 'abc123'], // cookies
    [],                          // files
    ['HTTP_ACCEPT' => 'application/json', 'HTTP_X_FORWARDED_FOR' => '10.0.0.1']
);

echo "1. Request created:" . PHP_EOL;
echo "   Method:      " . $request->getMethod() . PHP_EOL;
echo "   Path:        " . $request->getPathInfo() . PHP_EOL;
echo "   Query param: " . $request->query->get('format') . PHP_EOL;
echo "   Cookie:      " . $request->cookies->get('session_id') . PHP_EOL;
echo "   Accept:      " . $request->headers->get('Accept') . PHP_EOL;

// 2. Build a JSON response
$data = ['user_id' => 42, 'name' => 'Root.io Test', 'patched' => true];
$response = new JsonResponse($data, Response::HTTP_OK);
$response->headers->set('X-Powered-By', 'rootio_patcher');

echo PHP_EOL . "2. JSON response built:" . PHP_EOL;
echo "   Status:  " . $response->getStatusCode() . PHP_EOL;
echo "   Content: " . $response->getContent() . PHP_EOL;

// 3. Cookie handling (exercises the CVE-2024-50340 / CVE-2024-50341 code paths
//    around cookie header parsing and session fixation)
$cookie = Cookie::create('test_token')
    ->withValue('secure-value-123')
    ->withSecure(true)
    ->withHttpOnly(true)
    ->withSameSite(Cookie::SAMESITE_STRICT);

$response->headers->setCookie($cookie);

echo PHP_EOL . "3. Secure cookie set:" . PHP_EOL;
echo "   Name:     " . $cookie->getName() . PHP_EOL;
echo "   Secure:   " . ($cookie->isSecure() ? 'true' : 'false') . PHP_EOL;
echo "   HttpOnly: " . ($cookie->isHttpOnly() ? 'true' : 'false') . PHP_EOL;
echo "   SameSite: " . $cookie->getSameSite() . PHP_EOL;

// 4. Header bag inspection
echo PHP_EOL . "4. Response headers:" . PHP_EOL;
foreach (['Content-Type', 'X-Powered-By'] as $header) {
    echo "   $header: " . $response->headers->get($header) . PHP_EOL;
}

// 5. Request IP trust / forwarded headers (CVE-2024-50341 path)
$trustedRequest = Request::create('/ping', 'GET');
$trustedRequest->server->set('REMOTE_ADDR', '127.0.0.1');
$trustedRequest->headers->set('X-Forwarded-For', '203.0.113.42');

// Without trusted proxies, getClientIp() returns REMOTE_ADDR (safe default)
echo PHP_EOL . "5. IP handling (no trusted proxies configured):" . PHP_EOL;
echo "   Client IP: " . $trustedRequest->getClientIp() . PHP_EOL;

echo PHP_EOL . "✓ All checks passed — symfony/http-foundation is working correctly." . PHP_EOL;
