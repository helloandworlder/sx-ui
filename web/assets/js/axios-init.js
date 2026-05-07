axios.defaults.headers.post['Content-Type'] = 'application/x-www-form-urlencoded; charset=UTF-8';
axios.defaults.headers.common['X-Requested-With'] = 'XMLHttpRequest';

axios.interceptors.request.use(
    (config) => {
        if (config.data instanceof FormData) {
            config.headers['Content-Type'] = 'multipart/form-data';
        } else if (typeof config.url === 'string' && config.url.startsWith('/api/v1/')) {
            config.headers['Content-Type'] = 'application/json; charset=UTF-8';
            if (config.data !== undefined && typeof config.data !== 'string') {
                config.data = JSON.stringify(config.data);
            }
        } else {
            config.data = Qs.stringify(config.data, {
                arrayFormat: 'repeat',
            });
        }
        return config;
    },
    (error) => Promise.reject(error),
);

axios.interceptors.response.use(
    (response) => response,
    (error) => {
        if (error.response) {
            const statusCode = error.response.status;
            // Check the status code
            if (statusCode === 401) { // Unauthorized
                return window.location.reload();
            }
        }
        return Promise.reject(error);
    }
);
