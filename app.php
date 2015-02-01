<?php

/**
 * Add your routes here
 */
$app->get('/skins/{nickname}', function ($nickname) use ($app) {
    $systemVersion = $app->request->get("version", "int");
    $minecraftVersion = $app->request->get("minecraft_version", "string");

    // На всякий случай проверка на наличие .png для файла
    if (strrpos($nickname, ".png") != -1) {
        $nickname = explode(".", $nickname)[0];
    }

    // TODO: восстановить функцию деградации скинов

    $skin = Skins::findFirst(array(array(
        "nickname" => strtolower($nickname)
    )));

    if (!$skin || $skin->skinId == 0)
        return $app->response->redirect("http://skins.minecraft.net/MinecraftSkins/".$nickname.".png", true);

    return $app->response->redirect($skin->url);
})->setName("skinSystem");

$app->get('/cloaks/{nickname}', function ($nickname) use ($app) {
    // На всякий случай проверка на наличие .png для файла
    if (strrpos($nickname, ".png") != -1) {
        $nickname = explode(".", $nickname)[0];
    }

    return $app->response->redirect('http://skins.minecraft.net/MinecraftCloaks/'.$nickname.'.png');
});

$app->get("/textures/{nickname}", function($nickname) use ($app) {
    $skin = Skins::findFirst(array(array(
        "nickname" => strtolower($nickname)
    )));

    if ($skin && $skin->skinId != 0) {
        $url = $skin->url;
        $hash = $skin->hash;
    } else {
        $url = "http://skins.minecraft.net/MinecraftSkins/".$nickname.".png";
        $hash = md5("non-ely-".mktime(date("H"), 0, 0)."-".$nickname);
    }

    $textures = array(
        'SKIN' => array(
            'url' => $url,
            'hash' => $hash,
            'metadata' => array(
                'model' => ($skin && $skin->isSlim) ? "slim" : "default"
            )
        ),
        'CAPE' => array(
            'url' => '',
            'hash' => ''
        )
    );

    return $app->response->setJsonContent($textures);
});

/**
 * Должно проксироваться средствами nginx, но мало-ли
 */
$app->get("/minecraft.php", function() use ($app) {
    $nickname = $app->request->get("name", "string");
    $type = $app->request->get("type", "string");
    $minecraft_version = str_replace('_', '.',  $app->request->get("mine_ver", "string", NULL));
    $authlib_version = $app->request->get("auth_lib", "string", NULL);
    $version = $app->request->get("ver", "string");

    if ($version == "1_0_0")
        $version = "1";

    if ($type === "cloack" || $type === "cloak")
        return $app->response->redirect('http://skins.minecraft.net/MinecraftCloaks/'.$nickname.'.png');

    // Если запрос идёт через authlib, то мы не знаем версию Minecraft
    if ($authlib_version && !$minecraft_version) {
        $auth_to_mine = array(
            "1.3" => "1.7.2",
            "1.2" => "1.7.4",
            "1.3.1" => "1.7.5",
            "1.5.13" => "1.7.9",
            "1.5.16" => "1.7.10",
            "1.5.17" => "1.8.1"
        );

        if (array_key_exists($authlib_version, $auth_to_mine))
            $minecraft_version = $auth_to_mine[$authlib_version];
    }

    // Отправляем на новую систему скинов в правильном формате
    return $app->response->redirect($app->url->get(
        array(
            "for" => "skinSystem",
            "nickname" => $nickname
        ), array(
            "minecraft_version" => $minecraft_version,
            "version" => $version
        )
    ), true);
})->setName("fallbackSkinSystem");

$app->post("/system/setSkin", function() use ($app) {
    $headers = getallheaders();
    if (!array_key_exists("X-Ely-key", $headers) || $headers['X-Ely-key'] != "43fd2ce61b3f5704dfd729c1f2d6ffdb")
        return $app->response->setStatusCode(403, "Forbidden")->setContent("Хорошая попытка, мерзкий хакер.");

    $request = $app->request;
    $skin = Skins::findFirst(array(array(
        "userId" => (int) $request->getPost("userId", "int")
    )));

    if (!$skin) {
        $skin = new Skins();
        $skin->userId =  (int) $request->getPost("userId", "int");
    }

    $skin->nickname = strtolower($request->getPost("nickname", "string"));
    $skin->skinId = (int) $request->getPost("skinId", "int");
    $skin->hash = $request->getPost("hash", "string");
    $skin->is1_8 = (bool) $request->getPost("is1_8", "int");
    $skin->isSlim = (bool) $request->getPost("isSlim", "int");
    $skin->url = $request->getPost("url", "string");

    if ($skin->save())
        echo "OK";
    else
        echo "ERROR";
});

/**
 * Not found handler
 */
$app->notFound(function () use ($app) {
    $app->response
        ->setStatusCode(404, "Not Found")
        ->setContent('Not Found<br /> <a href="http://ely.by">Система скинов Ely.by</a>.')
        ->send();
});
