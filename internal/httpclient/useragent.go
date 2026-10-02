// Package httpclient agrupa configuracion HTTP compartida entre los
// distintos clientes del proyecto (yahoo, famafrench), para no duplicar
// valores que deben cambiar juntos.
package httpclient

// BrowserUserAgent se manda en todos los requests salientes del proyecto:
// tanto Yahoo Finance como la Kenneth French Data Library devuelven
// respuestas vacias o con error ante un User-Agent que no sea de navegador.
const BrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
